package driver

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"math"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/air-iot/json"
	"github.com/air-iot/logger"
	"github.com/shopspring/decimal"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/air-iot/sdk-go/v4/conn/mq"
	"github.com/air-iot/sdk-go/v4/driver/convert"
	"github.com/air-iot/sdk-go/v4/driver/entity"
	"github.com/air-iot/sdk-go/v4/utils/numberx"
)

type App interface {
	Start(Driver)
	WritePoints(context.Context, entity.Point) error
	WriteEvent(context.Context, entity.Event) error
	WriteWarning(context.Context, entity.Warn) error
	WriteWarningRecovery(ctx context.Context, tableId, dataId string, w entity.WarnRecovery) error
	FindDevice(ctx context.Context, table, id string, ret interface{}) error
	RunLog(context.Context, entity.Log) error
	UpdateTableData(ctx context.Context, table, id string, custom map[string]interface{}) error
	LogDebug(table, id string, msg interface{})
	LogInfo(table, id string, msg interface{})
	LogWarn(table, id string, msg interface{})
	LogError(table, id string, msg interface{})
	GetProjectId() string
	GetCommands(ctx context.Context, table, id string, ret interface{}) error
	UpdateCommand(ctx context.Context, id string, data entity.DriverInstruct) error
}

const (
	String  = "string"
	Float   = "float"
	Integer = "integer"
	Boolean = "boolean"
)

// app 数据采集类
type app struct {
	mq      mq.MQ
	stopped bool
	cli     *Client
	clean   func()

	cacheValue sync.Map
}

func Init() {
	// 设置随机数种子
	//rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())
	pflag.String("project", "default", "项目id")
	pflag.String("serviceId", "", "服务id")
	pflag.String("groupId", "", "组id")
	cfgPath := pflag.String("config", "./etc/", "配置文件")
	viper.SetDefault("log.level", 4)
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("mq.type", "mqtt")
	viper.SetDefault("mq.timeout", "60s")
	viper.SetDefault("mq.mqtt.host", "mqtt")
	viper.SetDefault("mq.mqtt.port", 1883)
	viper.SetDefault("mq.mqtt.username", "admin")
	viper.SetDefault("mq.mqtt.password", "public")
	viper.SetDefault("mq.mqtt.keepAlive", 60)
	viper.SetDefault("mq.mqtt.connectTimeout", 20)
	viper.SetDefault("mq.mqtt.protocolVersion", 4)
	viper.SetDefault("mq.rabbit.host", "rabbit")
	viper.SetDefault("mq.rabbit.port", 5672)
	viper.SetDefault("mq.rabbit.username", "admin")
	viper.SetDefault("mq.rabbit.password", "public")
	viper.SetDefault("mq.kafka.brokers", []string{"kafka:9092"})
	viper.SetDefault("driverGrpc.host", "driver")
	viper.SetDefault("driverGrpc.port", 9224)
	viper.SetDefault("driverGrpc.health.requestTime", "10s")
	viper.SetDefault("driverGrpc.health.retry", 3)
	viper.SetDefault("driverGrpc.waitTime", "5s")
	viper.SetDefault("driverGrpc.timeout", "600s")
	viper.SetDefault("driverGrpc.limit", 100)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	pflag.Parse()
	viper.AddConfigPath(*cfgPath)
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		panic(fmt.Errorf("读取命令行参数错误: %w", err))
	}
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置错误: %w", err))
	}
	if err := viper.Unmarshal(Cfg); err != nil {
		panic(fmt.Errorf("配置解析错误: %w", err))
	}
}

// NewApp 创建App
func NewApp() App {
	Init()
	a := new(app)
	if Cfg.Project == "" {
		panic("项目id未配置或未传参")
	}
	if Cfg.ServiceID == "" {
		panic("服务id未配置或未传参")
	}
	if Cfg.Driver.ID == "" || Cfg.Driver.Name == "" {
		panic("驱动id或name不能为空")
	}
	Cfg.Log.Syslog.ProjectId = Cfg.Project
	Cfg.Log.Syslog.ServiceName = fmt.Sprintf("%s-%s-%s", Cfg.Project, Cfg.ServiceID, Cfg.Driver.ID)
	logger.InitLogger(Cfg.Log)
	logger.Infof("启动配置=%+v", *Cfg)
	mqConn, clean, err := mq.NewMQ(Cfg.MQ)
	if err != nil {
		panic(fmt.Errorf("初始化消息队列错误: %w", err))
	}
	a.mq = mqConn
	a.clean = func() {
		clean()
	}
	a.cacheValue = sync.Map{}
	if Cfg.Pprof.Enable {
		go func() {
			//  路径/debug/pprof/
			addr := net.JoinHostPort(Cfg.Pprof.Host, Cfg.Pprof.Port)
			logger.Infof("pprof启动: 地址=%s", addr)
			if err := http.ListenAndServe(addr, nil); err != nil {
				logger.Errorf("pprof启动: 地址=%s. %v", addr, err)
				return
			}
		}()
	}
	return a
}

// Start 开始服务
func (a *app) Start(driver Driver) {
	a.stopped = false
	cli := Client{cacheConfig: sync.Map{}, cacheConfigNum: sync.Map{}}
	a.cli = cli.Start(a, driver)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	sig := <-ch
	close(ch)
	if err := driver.Stop(context.Background(), a); err != nil {
		logger.Warnf("驱动停止: %v", err.Error())
	}
	cli.Stop()
	a.stop()
	logger.Debugf("关闭服务: 信号=%v", sig)
	os.Exit(0)
}

// Stop 服务停止
func (a *app) stop() {
	a.stopped = true
	if a.clean != nil {
		a.clean()
	}
}

func (a *app) GetProjectId() string {
	return Cfg.Project
}

// WritePoints 写数据点数据
func (a *app) WritePoints(ctx context.Context, p entity.Point) error {
	//ctx = logger.NewModuleContext(ctx, entity.MODULE_WRITEPOINT)
	tableId := p.Table
	if tableId == "" {
		tableIdI, ok := a.cli.cacheConfig.Load(p.ID)
		if !ok {
			return fmt.Errorf("传入表id为空且未在配置中找到")
		}
		devI, ok := a.cli.cacheConfigNum.Load(p.ID)
		if ok {
			devM, _ := devI.(map[string]interface{})
			if len(devM) >= 2 {
				return fmt.Errorf("传入表id为空且在配置中找到多个表id")
			}
		}
		tableId = tableIdI.(string)
	}
	if p.ID == "" {
		return fmt.Errorf("设备id为空")
	}
	if p.Fields == nil || len(p.Fields) == 0 {
		return fmt.Errorf("采集数据有空值")
	}
	ctx = logger.NewTableContext(ctx, tableId)
	if Cfg.GroupID != "" {
		ctx = logger.NewGroupContext(ctx, Cfg.GroupID)
	}
	return a.writePoints(ctx, tableId, p)
}

func (a *app) writePoints(ctx context.Context, tableId string, p entity.Point) error {
	ctxTimeout, cancelTimeout := context.WithTimeout(context.Background(), Cfg.MQ.Timeout)
	defer cancelTimeout()
	fields := make(map[string]interface{})
	newLogger := logger.WithContext(ctx)
	for _, field := range p.Fields {
		if field.Value == nil {
			newLogger.Warnf("存数据点: 设备表=%s,设备=%s. 设备数据点值为空", tableId, p.ID)
			continue
		}
		//tagByte, err := json.Marshal(field.Tag)
		//if err != nil {
		//	newLogger.Warnf("表 %s 设备 %s 数据点序列化错误: %v", tableId, p.ID, err)
		//	continue
		//}
		//
		//tag := new(entity.Tag)
		//err = json.Unmarshal(tagByte, tag)
		//if err != nil {
		//	newLogger.Errorf("表 %s 设备 %s 数据点序列化tag结构体错误: %v", tableId, p.ID, err)
		//	continue
		//}
		tag := field.Tag
		if strings.TrimSpace(tag.ID) == "" {
			newLogger.Errorf("存数据点: 设备表=%s,设备=%s. 设备数据点标识为空", tableId, p.ID)
			continue
		}

		var value decimal.Decimal
		switch valueTmp := field.Value.(type) {
		case float32:
			if math.IsNaN(float64(valueTmp)) || math.IsInf(float64(valueTmp), 0) {
				//fields[tag.ID] = valueTmp
				newLogger.Errorf("存数据点: 设备表=%s,设备=%s,数据点=%s,值=%f. 设备数据点值不合法", tableId, p.ID, tag.ID, valueTmp)
				continue
			}
			value = decimal.NewFromFloat32(valueTmp)
		case float64:
			if math.IsNaN(valueTmp) || math.IsInf(valueTmp, 0) {
				//fields[tag.ID] = valueTmp
				newLogger.Errorf("存数据点: 设备表=%s,设备=%s,数据点=%s,值=%f. 设备数据点值不合法", tableId, p.ID, tag.ID, valueTmp)
				continue
			}
			value = decimal.NewFromFloat(valueTmp)
		case uint:
			value = decimal.NewFromInt(int64(valueTmp))
		case uint8:
			value = decimal.NewFromInt(int64(valueTmp))
		case uint16:
			value = decimal.NewFromInt(int64(valueTmp))
		case uint32:
			value = decimal.NewFromInt(int64(valueTmp))
		case uint64:
			value = decimal.NewFromInt(int64(valueTmp))
		case int:
			value = decimal.NewFromInt(int64(valueTmp))
		case int8:
			value = decimal.NewFromInt(int64(valueTmp))
		case int16:
			value = decimal.NewFromInt(int64(valueTmp))
		case int32:
			value = decimal.NewFromInt32(valueTmp)
		case int64:
			value = decimal.NewFromInt(valueTmp)
		case []byte:
			fields[tag.ID] = fmt.Sprintf("hex__%s", hex.EncodeToString(valueTmp))
			continue
		default:
			valTmp, err := numberx.GetValueByType("", field.Value)
			if err != nil {
				errCtx := logger.NewErrorContext(ctx, err)
				logger.WithContext(errCtx).Errorf("存数据点: 设备表=%s,设备=%s,数据点=%s. 设备数据点转类型失败", tableId, p.ID, tag.ID)
				continue
			}
			fields[tag.ID] = valTmp
			continue
		}
		val := convert.Value(&tag, value)
		cacheKey := fmt.Sprintf("%s__%s__%s", tableId, p.ID, tag.ID)
		preValF, ok := a.cacheValue.Load(cacheKey)
		var preVal *decimal.Decimal
		if ok {
			preF, ok := preValF.(*float64)
			if ok && preF != nil {
				preValue := decimal.NewFromFloat(*preF)
				preVal = &preValue
			}
		}
		newVal, rawVal, invalidType, save := convert.Range(tag.Range, preVal, &val)
		if newVal != nil {
			valTmp, err := numberx.GetValueByType("", newVal)
			if err != nil {
				errCtx := logger.NewErrorContext(ctx, err)
				logger.WithContext(errCtx).Errorf("存数据点: 设备表=%s,设备=%s,数据点=%s. 设备数据点转类型失败", tableId, p.ID, tag.ID)
			} else {
				fields[tag.ID] = valTmp
				if save {
					a.cacheValue.Store(cacheKey, newVal)
				}
			}
		}
		if rawVal != nil {
			valTmp, err := numberx.GetValueByType("", rawVal)
			if err != nil {
				errCtx := logger.NewErrorContext(ctx, err)
				logger.WithContext(errCtx).Errorf("存数据点: 设备表=%s,设备=%s,数据点=%s. 设备原始数据点转类型失败", tableId, p.ID, tag.ID)
			} else {
				fields[fmt.Sprintf("%s__invalid", tag.ID)] = valTmp
			}
		}
		if invalidType != "" {
			fields[fmt.Sprintf("%s__invalid__type", tag.ID)] = invalidType
		}
	}
	if len(fields) == 0 {
		return errors.New("数据点为空值")
	}
	if p.UnixTime == 0 {
		p.UnixTime = time.Now().Local().UnixMilli()
	}
	b, err := json.Marshal(&entity.WritePoint{ID: p.ID, CID: p.CID, Source: "device", UnixTime: p.UnixTime, Fields: fields, FieldTypes: p.FieldTypes})
	if err != nil {
		return err
	}
	if logger.IsLevelEnabled(logger.DebugLevel) {
		newLogger.Debugf("存数据点: 设备表=%s,设备=%s,数据=%s. 保存数据成功", tableId, p.ID, string(b))
	}
	return a.mq.Publish(ctxTimeout, []string{"data", Cfg.Project, tableId, p.ID}, b)
	//return nil
}

func (a *app) WriteWarning(ctx context.Context, w entity.Warn) error {
	//ctx = logger.NewModuleContext(ctx, entity.MODULE_WARN)
	tableId := w.TableId
	if tableId == "" {
		tableIdI, ok := a.cli.cacheConfig.Load(w.TableDataId)
		if !ok {
			return fmt.Errorf("传入表id为空且未在配置中找到")
		}
		devI, ok := a.cli.cacheConfigNum.Load(w.TableDataId)
		if ok {
			devM, _ := devI.(map[string]interface{})
			if len(devM) >= 2 {
				return fmt.Errorf("传入表id为空且在配置中找到多个表id")
			}
		}
		tableId = tableIdI.(string)
	}
	w.TableId = tableId
	if w.TableDataId == "" {
		return fmt.Errorf("设备id为空")
	}
	if tableId == "" {
		return fmt.Errorf("表id为空")
	}
	ctx = logger.NewTableContext(ctx, tableId)
	if Cfg.GroupID != "" {
		ctx = logger.NewGroupContext(ctx, Cfg.GroupID)
	}
	if w.Time == nil {
		n := time.Now().Local()
		w.Time = &n
	}

	wt := entity.WarnSend{
		ID:          w.ID,
		Table:       entity.Table{ID: tableId},
		TableData:   entity.TableData{ID: w.TableDataId},
		Level:       w.Level,
		Ruleid:      w.Ruleid,
		Fields:      w.Fields,
		WarningType: w.WarningType,
		Processed:   w.Processed,
		Time:        w.Time.Format(time.RFC3339),
		Alert:       w.Alert,
		Status:      w.Status,
		Handle:      w.Handle,
		Desc:        w.Desc,
	}
	b, err := json.Marshal(wt)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Cfg.MQ.Timeout)
	defer cancel()
	return a.mq.Publish(ctx, []string{"warningStorage", Cfg.Project, tableId, w.TableDataId}, b)
}

// WriteWarningRecovery 报警恢复
func (a *app) WriteWarningRecovery(ctx context.Context, tableId, dataId string, w entity.WarnRecovery) error {
	//ctx = logger.NewModuleContext(ctx, entity.MODULE_WARN)
	if tableId == "" {
		return fmt.Errorf("表id为空")
	}
	if dataId == "" {
		return fmt.Errorf("设备id为空")
	}
	if len(w.ID) == 0 {
		return fmt.Errorf("报警id为空")
	}
	ctx = logger.NewTableContext(ctx, tableId)
	if Cfg.GroupID != "" {
		ctx = logger.NewGroupContext(ctx, Cfg.GroupID)
	}
	if w.Data.Time == nil {
		n := time.Now().Local()
		w.Data.Time = &n
	}
	wt := entity.WarnRecoverySend{
		ID: w.ID,
		Data: entity.WarnRecoveryDataSend{
			Time:   w.Data.Time.Format(time.RFC3339),
			Fields: w.Data.Fields,
		},
	}
	b, err := json.Marshal(wt)
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), Cfg.MQ.Timeout)
	defer cancel()
	return a.mq.Publish(ctx, []string{"warningUpdate", Cfg.Project, tableId, dataId}, b)
}

func (a *app) WriteEvent(ctx context.Context, event entity.Event) error {
	return a.cli.WriteEvent(ctx, event)
}

func (a *app) FindDevice(ctx context.Context, table, id string, ret interface{}) error {
	return a.cli.FindDevice(ctx, table, id, ret)
}

func (a *app) GetCommands(ctx context.Context, table, id string, ret interface{}) error {
	return a.cli.GetCommands(ctx, table, id, ret)
}

func (a *app) UpdateCommand(ctx context.Context, id string, data entity.DriverInstruct) error {
	return a.cli.UpdateCommand(ctx, id, data)
}

func (a *app) RunLog(ctx context.Context, l entity.Log) error {
	return a.cli.RunLog(ctx, l)
}

func (a *app) UpdateTableData(ctx context.Context, table, id string, custom map[string]interface{}) error {
	return a.cli.UpdateTableData(ctx, entity.TableData{
		TableID: table,
		ID:      id,
		Data:    custom,
	}, &map[string]interface{}{})
}

// Log 写日志数据
func (a *app) Log(topic string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if err := a.mq.Publish(context.Background(), []string{"logs", topic}, b); err != nil {
		return
	}
}

// LogDebug 写日志数据
func (a *app) LogDebug(table, id string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if err := a.mq.Publish(context.Background(), []string{"logs", Cfg.Project, "debug", table, id}, b); err != nil {
		return
	}
}

// LogInfo 写日志数据
func (a *app) LogInfo(table, id string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if err := a.mq.Publish(context.Background(), []string{"logs", Cfg.Project, "info", table, id}, b); err != nil {
		return
	}
}

// LogWarn 写日志数据
func (a *app) LogWarn(table, id string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if err := a.mq.Publish(context.Background(), []string{"logs", Cfg.Project, "warn", table, id}, b); err != nil {
		return
	}
	return
}

// LogError 写日志数据
func (a *app) LogError(table, id string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	if err := a.mq.Publish(context.Background(), []string{"logs", Cfg.Project, "error", table, id}, b); err != nil {
		return
	}
	return
}
