package data_relay

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/air-iot/errors"
	"github.com/air-iot/json"
	"github.com/air-iot/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/air-iot/api-client-go/v4/datarelay"
	dGrpc "github.com/air-iot/sdk-go/v4/data_relay/grpc"
)

type Client struct {
	lock sync.RWMutex

	conn        *grpc.ClientConn
	cli         pb.DataRelayInstanceServiceClient
	app         App
	service     DataRelay
	clean       func()
	streamCount int32
}

const totalStream = 2

func (c *Client) Start(app App, service DataRelay) *Client {
	c.app = app
	c.service = service
	c.streamCount = 0
	c.start()
	return c
}

func (c *Client) start() {
	ctx := logger.NewModuleContext(context.Background(), MODULE_STARTSERVICE)
	ctx, cancel := context.WithCancel(ctx)
	c.clean = func() {
		cancel()
	}
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				waitTime := Cfg.DataRelayGrpc.WaitTime
				if err := c.run(ctx); err != nil {
					logger.WithContext(ctx).Errorln(err)
				}
				time.Sleep(waitTime)
			}
		}
	}()

}

func (c *Client) Stop() {
	ctx := logger.NewModuleContext(context.Background(), MODULE_STARTSERVICE)

	logger.WithContext(ctx).Infof("停止连接")
	if c.clean != nil {
		c.clean()
	}
	c.close(ctx)
}

func (c *Client) run(ctx context.Context) error {
	if err := c.connDriver(ctx); err != nil {
		return err
	}
	defer c.close(ctx)
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	c.startSteam(ctx)
	c.healthCheck(ctx)
	return nil
}

func (c *Client) close(ctx context.Context) {
	c.lock.Lock()
	defer c.lock.Unlock()
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			logger.WithContext(ctx).Errorf("关闭grpc连接. %v", err)
		}
	}
}

func (c *Client) connDriver(ctx context.Context) error {
	c.lock.Lock()
	defer c.lock.Unlock()
	ctx, cancel := context.WithTimeout(ctx, Cfg.DataRelayGrpc.Timeout)
	defer cancel()
	logger.WithContext(ctx).Infof("连接数据中转服务: 配置=%+v", Cfg.DataRelayGrpc)
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf("%s:%d", Cfg.DataRelayGrpc.Host, Cfg.DataRelayGrpc.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(Cfg.DataRelayGrpc.Limit*1024*1024), grpc.MaxCallSendMsgSize(Cfg.DataRelayGrpc.Limit*1024*1024)),
	)
	if err != nil {
		return fmt.Errorf("grpc.Dial error: %w", err)
	}
	c.conn = conn
	c.cli = pb.NewDataRelayInstanceServiceClient(conn)
	return nil
}

func (c *Client) healthCheck(ctx context.Context) {
	logger.WithContext(ctx).Infof("健康检查: 启动")
	nextTime := time.Now().Local().Add(Cfg.DataRelayGrpc.WaitTime * time.Duration(Cfg.DataRelayGrpc.Health.Retry))
	for {
		select {
		case <-ctx.Done():
			logger.WithContext(ctx).Infof("健康检查: 停止")
			return
		default:
			waitTime := Cfg.DataRelayGrpc.WaitTime
			ctx1 := logger.NewModuleContext(ctx, MODULE_HEALTHCHECK)

			newLogger := logger.WithContext(ctx1)
			newLogger.Debugf("健康检查: 开始")
			retry := Cfg.DataRelayGrpc.Health.Retry
			state := false
			for retry >= 0 {
				healthRes, err := c.healthRequest(ctx)
				if err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("健康检查: 健康检查第 %d 次错误", Cfg.DataRelayGrpc.Health.Retry-retry+1)
					state = true
					time.Sleep(waitTime)
				} else {
					state = false
					if healthRes.GetStatus() == pb.HealthCheckResponse_SERVING {
						newLogger.Debugf("健康检查: 正常")
						if healthRes.Errors != nil && len(healthRes.Errors) > 0 {
							for _, e := range healthRes.Errors {
								newLogger.Errorf("健康检查: code=%s,错误=%s", e.Code.String(), e.Message)
							}
						}
					} else if healthRes.GetStatus() == pb.HealthCheckResponse_SERVICE_UNKNOWN {
						newLogger.Errorf("健康检查: 服务端未找到本驱动服务")
						state = true
					}
					break
				}
				retry--
			}

			if state {
				return
			} else if time.Now().Local().After(nextTime) {
				nextTime = time.Now().Local().Add(time.Duration(Cfg.DataRelayGrpc.Health.Retry) * waitTime)
				getV := atomic.LoadInt32(&c.streamCount)
				newLogger.Debugf("健康检查: 找到流数量=%d", getV)
				if getV < totalStream {
					newLogger.Errorf("健康检查: 找到流数量不匹配,应为=%d,实际为=%d", totalStream, getV)
					return
				}
			}
			time.Sleep(waitTime)
		}
	}

}

func (c *Client) healthRequest(ctx context.Context) (*pb.HealthCheckResponse, error) {
	reqCtx, reqCancel := context.WithTimeout(ctx, Cfg.DataRelayGrpc.Health.RequestTime)
	defer reqCancel()
	healthRes, err := c.cli.HealthCheck(reqCtx, &pb.HealthCheckRequest{Service: Cfg.ServiceID, ProjectId: Cfg.Project, Type: Cfg.Service.ID})
	return healthRes, err
}

func (c *Client) startSteam(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("start: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				newCtx = logger.NewModuleContext(newCtx, MODULE_START)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("start: 启动stream")
				if err := c.StartStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("start: stream创建错误")
				}
				time.Sleep(Cfg.DataRelayGrpc.WaitTime)
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("httpProxy: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				newCtx = logger.NewModuleContext(newCtx, MODULE_HTTPPROXY)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("httpProxy: 启动stream")
				if err := c.HttpProxyStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("httpProxy: stream创建错误")
				}
				time.Sleep(Cfg.DataRelayGrpc.WaitTime)
			}
		}
	}()
}

func (c *Client) StartStream(ctx context.Context) error {
	stream, err := c.cli.StartStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Service.ID, Cfg.Service.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("start: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("start: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		ctx1 := logger.NewModuleContext(context.Background(), MODULE_START)
		logger.WithContext(ctx1).Debugf("start: 接收到开始请求")
		go func(res *pb.DataRelayInstanceStartRequest) {
			newCtx, cancel := context.WithTimeout(ctx1, Cfg.DataRelayGrpc.Timeout)
			defer cancel()
			defer func() {
				if errR := recover(); errR != nil {
					var errStr string
					switch v := errR.(type) {
					case error:
						errStr = v.Error()
						logger.Errorf("%+v", errors.WithStack(v))
					default:
						errStr = fmt.Sprintf("%v", v)
						logger.Errorln(v)
					}
					if err := stream.Send(&pb.Result{
						Request: res.Request,
						Status:  false,
						Info:    "执行错误",
						Detail:  errStr,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("start: 启动服务结果返回到数据中转服务错误")
					}
				}
			}()
			startRes := &pb.Result{
				Request: res.Request,
			}
			if err := c.service.Start(newCtx, c.app, res.GetData()); err != nil {
				startRes.Detail = err.Error()
				startRes.Info = "执行错误"
				startRes.Status = false
			} else {
				startRes.Status = true
				startRes.Info = "ok"
			}
			if err := stream.Send(startRes); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("start: 启动服务结果返回到数据中转服务错误")
			}
		}(res)
	}
}

func (c *Client) HttpProxyStream(ctx context.Context) error {
	stream, err := c.cli.HttpProxyStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Service.ID, Cfg.Service.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("httpProxy: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("httpProxy: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.HttpProxyRequest) {
			var header http.Header
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DataRelayGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewModuleContext(newCtx, MODULE_HTTPPROXY)
			logger.WithContext(newCtx).Debugf("httpProxy: type=%s,header=%s,请求数据=%s", res.Type, res.Headers, res.Data)
			defer func() {
				if errR := recover(); errR != nil {
					var errStr string
					switch v := errR.(type) {
					case error:
						errStr = v.Error()
						logger.Errorf("%+v", errors.WithStack(v))
					default:
						errStr = fmt.Sprintf("%v", v)
						logger.Errorln(v)
					}
					if err := stream.Send(&pb.Result{
						Request: res.Request,
						Status:  false,
						Info:    errStr,
						Detail:  errStr,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("httpProxy: 启动服务结果返回到数据中转服务错误")
					}
				}
			}()
			gr := &pb.Result{
				Request: res.Request,
			}
			if res.GetHeaders() != nil {
				if err := json.Unmarshal(res.GetHeaders(), &header); err != nil {
					gr.Info = fmt.Sprintf("解析请求头错误")
					gr.Detail = err.Error()
					gr.Status = false
				} else {
					runRes, err := c.service.HttpProxy(newCtx, c.app, res.GetType(), header, res.GetData())
					if err != nil {
						gr.Info = fmt.Sprintf("执行请求错误")
						gr.Detail = err.Error()
						gr.Status = false
					} else {
						gr.Result = runRes
						gr.Status = true
						gr.Info = "ok"
					}
				}
			} else {
				runRes, err := c.service.HttpProxy(newCtx, c.app, res.GetType(), header, res.GetData())
				if err != nil {
					gr.Info = fmt.Sprintf("执行请求错误")
					gr.Detail = err.Error()
					gr.Status = false
				} else {
					gr.Result = runRes
					gr.Status = true
					gr.Info = "ok"
				}
			}
			if err := stream.Send(gr); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("httpProxy: 请求结果返回到数据中转服务错误")
			}
		}(res)
	}
}
