package flow

import (
	"context"
	"fmt"
	"time"

	pb "github.com/air-iot/api-client-go/v4/engine"
	"github.com/air-iot/errors"
	"github.com/air-iot/json"
	"github.com/air-iot/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const (
	wait = 5
)

type Client struct {
	conn             *grpc.ClientConn
	cli              pb.PluginServiceClient
	app              App
	flow             Flow
	cleanStream      func()
	cleanHealthCheck func()
}

func (c *Client) Start(app App, flow Flow) *Client {
	c.app = app
	c.flow = flow
	if err := c.connFlow(); err != nil {
		logger.Errorln(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ctxHealth, cancelHealth := context.WithCancel(context.Background())
	c.cleanStream = func() {
		cancel()
	}
	c.cleanHealthCheck = func() {
		cancelHealth()
	}
	c.healthCheck(ctxHealth)
	c.startSteam(ctx)
	return c
}

func (c *Client) connFlow() error {
	logger.Infof("连接流程引擎: 配置=%+v", Cfg.FlowEngine)
	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", Cfg.FlowEngine.Host, Cfg.FlowEngine.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(Cfg.FlowEngine.Limit*1024*1024), grpc.MaxCallSendMsgSize(Cfg.FlowEngine.Limit*1024*1024)),
	)
	if err != nil {
		return fmt.Errorf("grpc.Dial error: %s", err)
	}
	cli := pb.NewPluginServiceClient(conn)
	c.conn = conn
	c.cli = cli
	return nil
}

func (c *Client) healthCheck(ctx context.Context) {
	logger.Infof("健康检查: 启动")
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Infof("健康检查: 结束")
				return
			default:
				newCtx := logger.NewModuleContext(context.Background(), MODULE_HEALTHCHECK)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("健康检查: 开始")
				retry := 3
				state := false
				for retry >= 0 {
					healthRes, err := c.cli.HealthCheck(ctx, &pb.HealthCheckRequest{Name: Cfg.Flow.Name})
					if err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("健康检查: 健康检查重试错误")
						state = true
						time.Sleep(time.Second * time.Duration(wait))
					} else {
						state = false
						if healthRes.GetStatus() == pb.HealthCheckResponse_SERVING {
							if healthRes.Errors != nil && len(healthRes.Errors) > 0 {
								for _, e := range healthRes.Errors {
									newLogger.Errorf("健康检查: code=%s,错误=%s", e.Code.String(), e.Message)
								}
							}
						}
						break
					}
					retry--
				}
				if state {
					c.cleanStream()
					if c.conn != nil {
						if err := c.conn.Close(); err != nil {
							errCtx := logger.NewErrorContext(newCtx, err)
							logger.WithContext(errCtx).Errorf("健康检查: 关闭连接")
						}
					}
					if err := c.connFlow(); err != nil {
						newLogger.Errorln(err)
					}
					ctx1, cancel1 := context.WithCancel(context.Background())
					c.startSteam(ctx1)
					c.cleanStream = func() {
						cancel1()
					}
				}
				time.Sleep(time.Second * time.Duration(wait))
			}
		}
	}()
}

func (c *Client) startSteam(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Infof("handler: 通过上下文关闭stream检查")
				return
			default:
				ctx1 := context.Background()
				ctx1 = logger.NewModuleContext(ctx1, MODULE_HANDLER)
				newLogger := logger.WithContext(ctx1)
				newLogger.Infof("handler: 启动stream")
				if err := c.Handler(ctx1); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("handler: stream创建错误")
				}
				time.Sleep(time.Second * time.Duration(wait))
			}
		}
	}()

	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.Infof("调试: 通过上下文关闭stream检查")
				return
			default:
				ctx1 := context.Background()
				ctx1 = logger.NewModuleContext(ctx1, MODULE_DEBUG)
				newLogger := logger.WithContext(ctx1)
				newLogger.Infof("调试: 启动stream")
				if err := c.DebugStream(ctx1); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("调试流: stream创建错误")
				}
				time.Sleep(time.Second * time.Duration(wait))
			}
		}
	}()
}

func (c *Client) Stop() {
	c.cleanStream()
	c.cleanHealthCheck()
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			logger.Errorf("grpc close error: %v", err)
		}
	}
}

func (c *Client) Handler(ctx context.Context) error {
	stream, err := c.cli.Register(GetGrpcContext(ctx, Cfg.Flow.Name, Cfg.Flow.Mode))
	if err != nil {
		return err
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("handler: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("handler: stream连接成功")
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.FlowRequest) {
			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Flow.Timeout))
			defer cancel()
			ctx1 = logger.NewModuleContext(ctx1, MODULE_HANDLER)
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
					gr := &pb.FlowResponse{
						ElementJob: res.ElementJob,
						Status:     false,
						Info:       errStr,
					}
					b, _ := json.Marshal(map[string]interface{}{})
					gr.Result = b
					if err := stream.Send(gr); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("handler: 执行结果返回到流程引擎错误")
					}
				}
			}()
			result, err := c.flow.Handler(ctx1, c.app, &Request{
				ProjectId:  res.ProjectId,
				FlowId:     res.FlowId,
				Job:        res.Job,
				ElementId:  res.ElementId,
				ElementJob: res.ElementJob,
				Config:     res.Config,
			})
			gr := &pb.FlowResponse{
				ElementJob: res.ElementJob,
			}
			if err != nil {
				gr.Status = false
				gr.Info = err.Error()
			} else {
				gr.Status = true
			}
			if result == nil {
				result = map[string]interface{}{}
			}
			b, _ := json.Marshal(result)
			gr.Result = b
			if err := stream.Send(gr); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("handler: 执行结果返回到流程引擎错误")
			}
		}(res)
	}
}

func (c *Client) DebugStream(ctx context.Context) error {
	stream, err := c.cli.DebugStream(GetGrpcContext(ctx, Cfg.Flow.Name, Cfg.Flow.Mode))
	if err != nil {
		return err
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("调试: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("调试: stream连接成功")
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.DebugRequest) {
			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Flow.Timeout))
			defer cancel()
			ctx1 = logger.NewModuleContext(ctx1, MODULE_DEBUG)
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
					gr := &pb.DebugResponse{
						ElementJob: res.ElementJob,
						Status:     false,
						Info:       errStr,
					}
					b, _ := json.Marshal(&DebugResult{Value: map[string]interface{}{}, Logs: make([]Syslog, 0)})
					gr.Result = b
					if err := stream.Send(gr); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("调试: 执行结果返回到流程引擎错误")
					}
				}
			}()
			result, err := c.flow.Debug(ctx1, c.app, &DebugRequest{
				ProjectId: res.ProjectId,
				FlowId:    res.FlowId,
				ElementId: res.ElementId,
				Config:    res.Config,
			})
			gr := &pb.DebugResponse{
				ElementJob: res.ElementJob,
			}
			if err != nil {
				gr.Status = false
				gr.Info = err.Error()
			} else {
				gr.Status = true
			}
			if result == nil {
				result = &DebugResult{Value: map[string]interface{}{}, Logs: make([]Syslog, 0)}
			}
			b, _ := json.Marshal(result)
			gr.Result = b
			if err := stream.Send(gr); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("调试: 执行结果返回到流程引擎错误")
			}
		}(res)
	}
}
