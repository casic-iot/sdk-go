package algorithm

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/air-iot/errors"
	"github.com/air-iot/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/air-iot/api-client-go/v4/algorithm"
	"github.com/air-iot/logger"
)

type Client struct {
	conn             *grpc.ClientConn
	cli              pb.AlgorithmServiceClient
	app              App
	algorithmService Service
	clean            func()
	cacheConfig      sync.Map
	cacheConfigNum   sync.Map
	//healthTime        time.Time
}

func (c *Client) Start(app App, algorithmService Service) *Client {
	c.app = app
	c.algorithmService = algorithmService
	ctx1 := logger.NewModuleContext(context.Background(), MODULE_START)
	err := algorithmService.Start(ctx1, app)
	if err != nil {
		panic(err.Error())
	}
	c.start()
	return c
}

func (c *Client) start() {
	if err := c.connAlgorithm(); err != nil {
		logger.Errorln(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	c.clean = func() {
		cancel()
	}
	c.healthCheck(ctx)
	c.startSteam(ctx)
}

func (c *Client) Stop() {
	if c.clean != nil {
		c.clean()
	}
	if c.conn != nil {
		if err := c.conn.Close(); err != nil {
			logger.Errorf("grpc close error: %s", err.Error())
		}
	}
}

func (c *Client) restart() {
	logger.Infof("重启算法管理连接")
	c.Stop()
	c.start()
}

func (c *Client) connAlgorithm() error {
	logger.Infof("连接算法管理: 配置=%+v", Cfg.AlgorithmGrpc)
	conn, err := grpc.NewClient(
		fmt.Sprintf("%s:%d", Cfg.AlgorithmGrpc.Host, Cfg.AlgorithmGrpc.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(Cfg.AlgorithmGrpc.Limit*1024*1024), grpc.MaxCallSendMsgSize(Cfg.AlgorithmGrpc.Limit*1024*1024)),
	)
	if err != nil {
		return fmt.Errorf("grpc.Dial error: %s", err)
	}
	cli := pb.NewAlgorithmServiceClient(conn)
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
				ctxHealth := logger.NewModuleContext(ctx, MODULE_HEALTHCHECK)
				logger.WithContext(ctxHealth).Infof("健康检查: 开始")
				retry := Cfg.AlgorithmGrpc.Health.Retry
				state := false
				for retry >= 0 {
					healthRes, err := c.healthRequest(ctx)
					if err != nil {
						logger.WithContext(logger.NewErrorContext(ctxHealth, err)).Errorf("健康检查: 健康检查请求错误")
						state = true
						time.Sleep(time.Second * time.Duration(Cfg.AlgorithmGrpc.WaitTime))
					} else {
						state = false
						if healthRes.GetStatus() == pb.HealthCheckResponse_SERVING {
							logger.WithContext(ctxHealth).Infof("健康检查: 正常")
							if healthRes.Errors != nil && len(healthRes.Errors) > 0 {
								for _, e := range healthRes.Errors {
									logger.WithContext(ctxHealth).Errorf("健康检查: code=%s,错误=%s", e.Code.String(), e.Message)
								}
							}
						} else if healthRes.GetStatus() == pb.HealthCheckResponse_SERVICE_UNKNOWN {
							logger.WithContext(ctxHealth).Errorf("健康检查: 服务端未找到本算法服务")
							state = true
						}
						break
					}
					retry--
				}
				if state {
					c.restart()
				}
				time.Sleep(time.Second * time.Duration(Cfg.AlgorithmGrpc.WaitTime))
			}
		}
	}()
}

func (c *Client) healthRequest(ctx context.Context) (*pb.HealthCheckResponse, error) {
	reqCtx, reqCancel := context.WithTimeout(ctx, time.Second*time.Duration(Cfg.AlgorithmGrpc.Health.RequestTime))
	defer reqCancel()
	healthRes, err := c.cli.HealthCheck(reqCtx, &pb.HealthCheckRequest{Service: Cfg.ServiceID})
	return healthRes, err
}

func (c *Client) startSteam(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("schema: 通过上下文关闭stream检查")
				return
			default:
				ctx1 := context.Background()
				newLogger := logger.WithContext(logger.NewModuleContext(ctx1, MODULE_SCHEMA))
				newLogger.Infof("schema流: 启动stream")
				if err := c.SchemaStream(ctx1); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("schema: stream创建错误")
				}
				time.Sleep(time.Second * time.Duration(Cfg.AlgorithmGrpc.WaitTime))
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("run: 通过上下文关闭stream检查")
				return
			default:
				ctx1 := context.Background()
				newLogger := logger.WithContext(logger.NewModuleContext(ctx1, MODULE_RUN))
				newLogger.Infof("run: 启动stream")
				if err := c.RunStream(context.Background()); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("run: stream创建错误")
				}
				time.Sleep(time.Second * time.Duration(Cfg.AlgorithmGrpc.WaitTime))
			}
		}
	}()
}

func (c *Client) SchemaStream(ctx context.Context) error {
	stream, err := c.cli.SchemaStream(GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Algorithm.ID, Cfg.Algorithm.Name))
	if err != nil {
		return err
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("schema: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("schema: stream连接成功")
	for {
		rec, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(req *pb.SchemaRequest) {
			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Algorithm.Timeout))
			defer cancel()
			ctx1 = logger.NewModuleContext(ctx1, MODULE_SCHEMA)
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
					schemaRes := new(grpcResult)
					schemaRes.Error = errStr
					schemaRes.Code = 400
					bts, _ := json.Marshal(schemaRes)
					if err := stream.Send(&pb.SchemaResult{
						Request: req.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("schema: 执行结果返回到算法服务错误")
					}
				}
			}()
			schema, err := c.algorithmService.Schema(ctx1, c.app, req.Lang)
			schemaRes := new(grpcResult)
			if err != nil {
				schemaRes.Error = err.Error()
				schemaRes.Code = 400
			} else {
				schemaRes.Result = schema
				schemaRes.Code = 200
			}
			bts, _ := json.Marshal(schemaRes)
			if err := stream.Send(&pb.SchemaResult{
				Request: req.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("schema: 执行结果返回到算法服务错误")
			}
		}(rec)
	}
}

func (c *Client) RunStream(ctx context.Context) error {
	stream, err := c.cli.RunStream(GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Algorithm.ID, Cfg.Algorithm.Name))
	if err != nil {
		return err
	}
	defer func() {
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("run: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("run: stream连接成功")
	for {
		res0, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.RunRequest) {

			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Algorithm.Timeout))
			defer cancel()
			ctx1 = logger.NewModuleContext(ctx1, MODULE_RUN)
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
					gr := new(grpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.RunResult{
						Request: res.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("run: 执行结果返回到算法服务错误")
					}
				}
			}()
			runRes, err := c.algorithmService.Run(ctx1, c.app, res.Data)
			gr := new(grpcResult)
			if err != nil {
				gr.Error = err.Error()
				gr.Code = 400
			} else {
				gr.Result = runRes
				gr.Code = 200
			}
			bts, _ := json.Marshal(gr)
			if err := stream.Send(&pb.RunResult{
				Request: res.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("run: 执行结果返回到算法服务错误")
			}
		}(res0)
	}
}
