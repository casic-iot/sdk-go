package driver

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/air-iot/errors"
	"github.com/air-iot/json"
	"github.com/air-iot/sdk-go/v4/driver/entity"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/air-iot/api-client-go/v4/driver"
	"github.com/air-iot/logger"
	dGrpc "github.com/air-iot/sdk-go/v4/driver/grpc"
)

type Client struct {
	lock sync.RWMutex

	conn           *grpc.ClientConn
	cli            pb.DriverServiceClient
	app            App
	driver         Driver
	clean          func()
	cacheConfig    sync.Map
	cacheConfigNum sync.Map
	streamCount    int32
}

const totalStream = 7

func (c *Client) Start(app App, driver Driver) *Client {
	c.app = app
	c.driver = driver
	c.streamCount = 0
	c.start()
	return c
}

func (c *Client) start() {
	ctx := logger.NewModuleContext(context.Background(), entity.MODULE_STARTDRIVER)
	if Cfg.GroupID != "" {
		ctx = logger.NewGroupContext(ctx, Cfg.GroupID)
	}
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
				waitTime := Cfg.DriverGrpc.WaitTime
				if err := c.run(ctx); err != nil {
					logger.WithContext(ctx).Errorln(err)
				}
				time.Sleep(waitTime)
			}
		}
	}()

}

func (c *Client) Stop() {
	ctx := logger.NewModuleContext(context.Background(), entity.MODULE_STARTDRIVER)
	if Cfg.GroupID != "" {
		ctx = logger.NewGroupContext(ctx, Cfg.GroupID)
	}
	logger.WithContext(ctx).Infof("停止驱动管理连接")
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
	ctx, cancel := context.WithTimeout(ctx, Cfg.DriverGrpc.Timeout)
	defer cancel()
	logger.WithContext(ctx).Infof("连接driver: 配置=%+v", Cfg.DriverGrpc)
	conn, err := grpc.DialContext(
		ctx,
		fmt.Sprintf("%s:%d", Cfg.DriverGrpc.Host, Cfg.DriverGrpc.Port),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.MaxCallRecvMsgSize(Cfg.DriverGrpc.Limit*1024*1024), grpc.MaxCallSendMsgSize(Cfg.DriverGrpc.Limit*1024*1024)),
	)
	if err != nil {
		return fmt.Errorf("grpc.Dial error: %w", err)
	}
	c.conn = conn
	c.cli = pb.NewDriverServiceClient(conn)
	return nil
}

func (c *Client) healthCheck(ctx context.Context) {
	logger.WithContext(ctx).Infof("健康检查: 启动")
	nextTime := time.Now().Local().Add(Cfg.DriverGrpc.WaitTime * time.Duration(Cfg.DriverGrpc.Health.Retry))
	for {
		select {
		case <-ctx.Done():
			logger.WithContext(ctx).Infof("健康检查: 停止")
			return
		default:
			waitTime := Cfg.DriverGrpc.WaitTime
			ctx1 := logger.NewModuleContext(ctx, entity.MODULE_HEALTHCHECK)
			if Cfg.GroupID != "" {
				ctx1 = logger.NewGroupContext(ctx1, Cfg.GroupID)
			}
			newLogger := logger.WithContext(ctx1)
			newLogger.Debugf("健康检查: 开始")
			retry := Cfg.DriverGrpc.Health.Retry
			state := false
			for retry >= 0 {
				healthRes, err := c.healthRequest(ctx)
				if err != nil {
					errCtx := logger.NewErrorContext(ctx1, err)
					logger.WithContext(errCtx).Errorf("健康检查: 健康检查第 %d 次错误", Cfg.DriverGrpc.Health.Retry-retry+1)
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
				nextTime = time.Now().Local().Add(time.Duration(Cfg.DriverGrpc.Health.Retry) * waitTime)
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
	reqCtx, reqCancel := context.WithTimeout(ctx, Cfg.DriverGrpc.Health.RequestTime)
	defer reqCancel()
	healthRes, err := c.cli.HealthCheck(reqCtx, &pb.HealthCheckRequest{Service: Cfg.ServiceID, ProjectId: Cfg.Project, DriverId: Cfg.Driver.ID})
	return healthRes, err
}

func (c *Client) WriteEvent(ctx context.Context, event entity.Event) error {
	if event.Table == "" || event.ID == "" || event.EventID == "" {
		return fmt.Errorf("表、设备或事件ID为空")
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	res, err := c.cli.Event(ctx, &pb.Request{
		Project: Cfg.Project,
		Data:    b,
	})
	if err != nil {
		return err
	}
	if !res.GetStatus() {
		return fmt.Errorf(res.GetInfo())
	}
	return nil
}

func (c *Client) FindDevice(ctx context.Context, table, id string, ret interface{}) error {
	if id == "" {
		return fmt.Errorf("设备ID为空")
	}
	res, err := c.cli.FindTableData(ctx, &pb.TableDataRequest{
		ProjectId:   Cfg.Project,
		DriverId:    Cfg.Driver.ID,
		Service:     Cfg.ServiceID,
		TableId:     table,
		TableDataId: id,
	})
	if err != nil {
		return err
	}
	if !res.GetStatus() {
		return fmt.Errorf(res.GetInfo())
	}
	if err := json.Unmarshal(res.GetResult(), ret); err != nil {
		return fmt.Errorf("解析请求结果错误: %v", err)
	}
	return nil
}

func (c *Client) RunLog(ctx context.Context, l entity.Log) error {
	if l.SerialNo == "" {
		return fmt.Errorf("流水号为空")
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	res, err := c.cli.CommandLog(ctx, &pb.Request{
		Project: Cfg.Project,
		Data:    b,
	})
	if err != nil {
		return err
	}
	if !res.GetStatus() {
		return fmt.Errorf(res.GetInfo())
	}
	return nil
}

func (c *Client) UpdateTableData(ctx context.Context, l entity.TableData, result interface{}) error {
	if l.TableID == "" || l.ID == "" {
		return fmt.Errorf("表或记录id为空")
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	res, err := c.cli.UpdateTableData(ctx, &pb.Request{
		Project: Cfg.Project,
		Data:    b,
	})
	if err != nil {
		return err
	}
	if !res.GetStatus() {
		return fmt.Errorf(res.GetInfo())
	}
	if err := json.Unmarshal(res.GetResult(), result); err != nil {
		return err
	}
	return nil
}

func (c *Client) startSteam(ctx context.Context) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("schema: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_SCHEMA)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("schema: 启动stream")
				if err := c.SchemaStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("schema: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("start: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_START)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("start: 启动stream")
				if err := c.StartStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("start: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("执行指令: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_RUN)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("执行指令: 启动stream")
				if err := c.RunStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("执行指令: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("写数据点: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_WRITETAG)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("写数据点: 启动stream")
				if err := c.WriteTagStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("写数据点: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("批量执行指令: stream创建错误")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_BATCHRUN)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("批量执行指令: 启动stream")
				if err := c.BatchRunStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("批量执行指令: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
	go func() {
		for {
			select {
			case <-ctx.Done():
				logger.WithContext(ctx).Infof("调试: 通过上下文关闭stream检查")
				return
			default:
				newCtx := context.WithoutCancel(ctx)
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_DEBUG)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("调试: 启动stream")
				if err := c.DebugStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("调试: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
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
				if Cfg.GroupID != "" {
					newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
				}
				newCtx = logger.NewModuleContext(newCtx, entity.MODULE_HTTPPROXY)
				newLogger := logger.WithContext(newCtx)
				newLogger.Infof("httpProxy: 启动stream")
				if err := c.HttpProxyStream(newCtx); err != nil {
					errCtx := logger.NewErrorContext(newCtx, err)
					logger.WithContext(errCtx).Errorf("httpProxy: stream创建错误")
				}
				time.Sleep(Cfg.DriverGrpc.WaitTime)
			}
		}
	}()
}

func (c *Client) SchemaStream(ctx context.Context) error {
	stream, err := c.cli.SchemaStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("schema: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("schema: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.SchemaRequest) {
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewModuleContext(newCtx, entity.MODULE_SCHEMA)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
			logger.WithContext(newCtx).Debugf("schema: 接收到查询请求")
			schema, err := c.driver.Schema(newCtx, c.app)
			schemaRes := new(entity.GrpcResult)
			if err != nil {
				schemaRes.Error = err.Error()
				schemaRes.Code = 400
			} else {
				schemaRes.Result = schema
				schemaRes.Code = 200
			}
			bts, _ := json.Marshal(schemaRes)
			if err := stream.Send(&pb.SchemaResult{
				Request: res.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("schema: 配置返回到驱动管理错误")
			}
		}(res)
	}
}

func (c *Client) StartStream(ctx context.Context) error {
	stream, err := c.cli.StartStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
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
		ctx1 := logger.NewModuleContext(context.Background(), entity.MODULE_START)
		logger.WithContext(ctx1).Debugf("start: 接收到开始请求")
		var cfg entity.Instance
		if err := json.Unmarshal(res.Config, &cfg); err != nil {
			startRes := new(entity.GrpcResult)
			startRes.Error = err.Error()
			startRes.Code = 400
			bts, _ := json.Marshal(startRes)
			if err := stream.Send(&pb.StartResult{
				Request: res.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(ctx1, err)
				logger.WithContext(errCtx).Errorf("start: 解析配置的错误返回到驱动管理错误")
			}
			continue
		}
		if cfg.Debug != nil {
			if *cfg.Debug {
				logger.SetLevel(logger.DebugLevel)
			} else {
				logger.SetLevel(logger.InfoLevel)
			}
		}
		c.cacheConfigNum = sync.Map{}
		c.cacheConfig = sync.Map{}
		if cfg.GroupId != "" {
			Cfg.GroupID = cfg.GroupId
		}
		if Cfg.GroupID != "" {
			ctx1 = logger.NewGroupContext(ctx1, Cfg.GroupID)
		}
		if cfg.Tables != nil {
			for _, t := range cfg.Tables {
				if t.Devices == nil {
					continue
				}
				for _, device := range t.Devices {
					devM, ok := c.cacheConfigNum.Load(device.Id)
					var devI map[string]interface{}
					if ok {
						devI, _ = devM.(map[string]interface{})
					} else {
						devI = map[string]interface{}{}
					}
					devI[t.Id] = struct{}{}
					c.cacheConfigNum.Store(device.Id, devI)
					c.cacheConfig.Store(device.Id, t.Id)
				}
			}
		}
		run := func(res *pb.StartRequest) {
			newCtx, cancel := context.WithTimeout(ctx1, Cfg.DriverGrpc.Timeout)
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
					startRes := new(entity.GrpcResult)
					startRes.Error = errStr
					startRes.Code = 400
					bts, _ := json.Marshal(startRes)
					if err := stream.Send(&pb.StartResult{
						Request: res.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("start: 启动驱动结果返回到驱动管理错误")
					}
				}
			}()
			startRes := new(entity.GrpcResult)
			if err := c.driver.Start(newCtx, c.app, res.Config); err != nil {
				startRes.Error = err.Error()
				startRes.Code = 400
			} else {
				startRes.Code = 200
			}
			bts, _ := json.Marshal(startRes)
			if err := stream.Send(&pb.StartResult{
				Request: res.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("start: 启动驱动结果返回到驱动管理错误")
			}
		}
		run(res)
	}
}

func (c *Client) RunStream(ctx context.Context) error {
	stream, err := c.cli.RunStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("执行指令: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("执行指令: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.RunRequest) {
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewTDMContext(newCtx, res.TableId, res.Id, entity.MODULE_RUN)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
			logger.WithContext(newCtx).Debugf("执行指令: 设备表=%s,设备=%s,指令=%s", res.TableId, res.Id, res.Command)
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
					gr := new(entity.GrpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.RunResult{
						Request: res.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("执行指令: 执行指令结果返回到驱动管理错误")
					}
				}
			}()
			gr := new(entity.GrpcResult)
			runRes, err := c.driver.Run(newCtx, c.app, &entity.Command{
				Table:    res.TableId,
				Id:       res.Id,
				SerialNo: res.SerialNo,
				Command:  res.Command,
			})
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
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("执行指令: 执行指令结果返回到驱动管理错误")
			}
		}(res)
	}
}

func (c *Client) WriteTagStream(ctx context.Context) error {
	stream, err := c.cli.WriteTagStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("写数据点: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("写数据点: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.RunRequest) {
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewTDMContext(newCtx, res.TableId, res.Id, entity.MODULE_WRITETAG)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
			logger.WithContext(newCtx).Debugf("写数据点: 设备表=%s,设备=%s,指令=%s", res.TableId, res.Id, res.Command)
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
					gr := new(entity.GrpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.RunResult{
						Request: res.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("写数据点: 写数据点执行结果返回到驱动管理错误")
					}
				}
			}()
			gr := new(entity.GrpcResult)
			runRes, err := c.driver.WriteTag(newCtx, c.app, &entity.Command{
				Table:    res.TableId,
				Id:       res.Id,
				SerialNo: res.SerialNo,
				Command:  res.Command,
			})
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
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("写数据点: 写数据点执行结果返回到驱动管理错误")
			}
		}(res)
	}
}

func (c *Client) BatchRunStream(ctx context.Context) error {
	stream, err := c.cli.BatchRunStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("批量执行指令: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("批量执行指令: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.BatchRunRequest) {
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewModuleContext(newCtx, entity.MODULE_BATCHRUN)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
			newCtx = logger.NewTableContext(newCtx, res.TableId)
			logger.WithContext(newCtx).Debugf("批量执行指令: 设备表=%s,设备=%+v,指令=%s", res.TableId, res.Id, res.Command)
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
					gr := new(entity.GrpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.BatchRunResult{
						Request: res.Request,
						Message: bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("批量执行指令: 批量执行指令结果返回到驱动管理错误")
					}
				}
			}()
			gr := new(entity.GrpcResult)
			runRes, err := c.driver.BatchRun(newCtx, c.app, &entity.BatchCommand{
				Table:    res.TableId,
				Ids:      res.Id,
				SerialNo: res.SerialNo,
				Command:  res.Command,
			})
			if err != nil {
				gr.Error = err.Error()
				gr.Code = 400
			} else {
				gr.Result = runRes
				gr.Code = 200
			}
			bts, _ := json.Marshal(gr)
			if err := stream.Send(&pb.BatchRunResult{
				Request: res.Request,
				Message: bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("批量执行指令: 批量执行指令结果返回到驱动管理错误")
			}
		}(res)
	}
}

func (c *Client) DebugStream(ctx context.Context) error {
	stream, err := c.cli.DebugStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
	if err != nil {
		return err
	}
	defer func() {
		atomic.AddInt32(&c.streamCount, -1)
		if err := stream.CloseSend(); err != nil {
			errCtx := logger.NewErrorContext(ctx, err)
			logger.WithContext(errCtx).Errorf("调试: stream关闭错误")
		}
	}()
	logger.WithContext(ctx).Infof("调试: stream连接成功")
	atomic.AddInt32(&c.streamCount, 1)
	for {
		res, err := stream.Recv()
		if err != nil {
			return err
		}
		go func(res *pb.Debug) {

			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewModuleContext(newCtx, entity.MODULE_DEBUG)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
			logger.WithContext(newCtx).Debugf("调试: 请求数据=%s", res.Data)
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
					gr := new(entity.GrpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.Debug{
						Request: res.Request,
						Data:    bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("调试: 调试结果返回到驱动管理错误")
					}
				}
			}()
			runRes, err := c.driver.Debug(newCtx, c.app, res.Data)
			gr := new(entity.GrpcResult)
			if err != nil {
				gr.Error = err.Error()
				gr.Code = 400
			} else {
				gr.Result = runRes
				gr.Code = 200
			}
			bts, _ := json.Marshal(gr)
			if err := stream.Send(&pb.Debug{
				Request: res.Request,
				Data:    bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("调试: 调试结果返回到驱动管理错误")
			}
		}(res)
	}
}

func (c *Client) HttpProxyStream(ctx context.Context) error {
	stream, err := c.cli.HttpProxyStream(dGrpc.GetGrpcContext(ctx, Cfg.ServiceID, Cfg.Project, Cfg.Driver.ID, Cfg.Driver.Name))
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
			newCtx, cancel := context.WithTimeout(context.Background(), Cfg.DriverGrpc.Timeout)
			defer cancel()
			newCtx = logger.NewModuleContext(newCtx, entity.MODULE_HTTPPROXY)
			if Cfg.GroupID != "" {
				newCtx = logger.NewGroupContext(newCtx, Cfg.GroupID)
			}
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
					gr := new(entity.GrpcResult)
					gr.Error = errStr
					gr.Code = 400
					bts, _ := json.Marshal(gr)
					if err := stream.Send(&pb.HttpProxyResult{
						Request: res.Request,
						Data:    bts,
					}); err != nil {
						errCtx := logger.NewErrorContext(newCtx, err)
						logger.WithContext(errCtx).Errorf("httpProxy: 请求结果返回到驱动管理错误")
					}
				}
			}()
			gr := new(entity.GrpcResult)
			if res.GetHeaders() != nil {
				if err := json.Unmarshal(res.GetHeaders(), &header); err != nil {
					gr.Error = fmt.Sprintf("httpProxy流错误:%v", err)
					gr.Code = 400
				} else {
					runRes, err := c.driver.HttpProxy(newCtx, c.app, res.GetType(), header, res.GetData())
					if err != nil {
						gr.Error = err.Error()
						gr.Code = 400
					} else {
						gr.Result = runRes
						gr.Code = 200
					}
				}
			} else {
				runRes, err := c.driver.HttpProxy(newCtx, c.app, res.GetType(), header, res.GetData())
				if err != nil {
					gr.Error = err.Error()
					gr.Code = 400
				} else {
					gr.Result = runRes
					gr.Code = 200
				}
			}
			bts, _ := json.Marshal(gr)
			if err := stream.Send(&pb.HttpProxyResult{
				Request: res.Request,
				Data:    bts,
			}); err != nil {
				errCtx := logger.NewErrorContext(newCtx, err)
				logger.WithContext(errCtx).Errorf("httpProxy: 请求结果返回到驱动管理错误")
			}
		}(res)
	}
}
