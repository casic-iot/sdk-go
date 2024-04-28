package flow_extionsion

import (
	"context"
	"fmt"
	"time"

	"github.com/air-iot/errors"
	"github.com/air-iot/json"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/air-iot/api-client-go/v4/engine"
	"github.com/air-iot/logger"
)

const (
	wait = 5
)

type Client struct {
	conn             *grpc.ClientConn
	cli              pb.ExtensionServiceClient
	app              App
	extension        Extension
	cleanStream      func()
	cleanHealthCheck func()
}

func (c *Client) Start(app App, extension Extension) *Client {
	c.app = app
	c.extension = extension
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
	logger.Infof("连接flow: 配置=%+v", Cfg.FlowEngine)
	conn, err := grpc.DialContext(
		context.Background(),
		fmt.Sprintf("%s:%d", Cfg.FlowEngine.Host, Cfg.FlowEngine.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return fmt.Errorf("grpc.Dial error: %s", err)
	}
	c.conn = conn
	c.cli = pb.NewExtensionServiceClient(conn)
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
					healthRes, err := c.cli.HealthCheck(ctx, &pb.ExtensionHealthCheckRequest{Id: Cfg.Extension.Id})
					if err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("健康检查: 健康检查重试错误")
						state = true
						time.Sleep(time.Second * time.Duration(wait))
					} else {
						state = false
						if healthRes.GetStatus() == pb.ExtensionHealthCheckResponse_SERVING {
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
						logger.Errorln(err)
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
				logger.WithContext(ctx).Infof("schema: 通过上下文关闭stream检查")
				return
			default:
				ctx1 := context.Background()
				ctx1 = logger.NewModuleContext(ctx1, MODULE_SCHEMA)
				newLogger := logger.WithContext(ctx1)
				newLogger.Infof("schema流: 启动stream")
				if err := c.Schema(context.Background()); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("schema: stream创建错误")
				}
				time.Sleep(time.Second * time.Duration(wait))
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
				ctx1 = logger.NewModuleContext(ctx1, MODULE_RUN)
				newLogger := logger.WithContext(ctx1)
				newLogger.Infof("run: 启动stream")
				if err := c.Run(context.Background()); err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("run: stream创建错误")
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

func (c *Client) Schema(ctx context.Context) error {
	stream, err := c.cli.SchemaStream(GetGrpcContext(ctx, Cfg.Extension.Id, Cfg.Extension.Name))
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
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.ExtensionSchemaRequest) {
			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Extension.Timeout))
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
					gr := &pb.ExtensionResult{
						Request: res.GetRequest(),
						Status:  false,
						Info:    errStr,
					}
					if err := stream.Send(gr); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("schema: 执行结果返回到流程扩展节点错误")
					}
				}
			}()
			result, err := c.extension.Schema(ctx1, c.app)
			gr := &pb.ExtensionResult{
				Request: res.GetRequest(),
			}
			if err != nil {
				gr.Status = false
				gr.Info = err.Error()
			} else {
				gr.Status = true
				gr.Result = []byte(result)
			}
			if err := stream.Send(gr); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("schema: 执行结果返回到流程扩展节点错误")
			}
		}(res)
	}
}

func (c *Client) Run(ctx context.Context) error {
	stream, err := c.cli.RunStream(GetGrpcContext(ctx, Cfg.Extension.Id, Cfg.Extension.Name))
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
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.ExtensionRunRequest) {
			ctx1, cancel := context.WithTimeout(context.Background(), time.Second*time.Duration(Cfg.Extension.Timeout))
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
					gr := &pb.ExtensionResult{
						Request: res.GetRequest(),
						Status:  false,
						Info:    errStr,
					}
					b, _ := json.Marshal(map[string]interface{}{})
					gr.Result = b
					if err := stream.Send(gr); err != nil {
						errCtx := logger.NewErrorContext(ctx1, err)
						logger.WithContext(errCtx).Errorf("run: 执行结果返回到流程扩展节点错误")
					}
				}
			}()
			result, err := c.extension.Run(ctx1, c.app, res.GetData())
			gr := &pb.ExtensionResult{
				Request: res.GetRequest(),
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
				logger.WithContext(errCtx).Errorf("run: 执行结果返回到流程扩展节点错误")
			}
		}(res)
	}
}
