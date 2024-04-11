package driver

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"math"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/air-iot/sdk-go/util/aaes"
	"github.com/air-iot/sdk-go/util/json"
	"github.com/duke-git/lancet/v2/cryptor"
	"github.com/google/uuid"
	gws "github.com/gorilla/websocket"
	"github.com/shopspring/decimal"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"

	"github.com/air-iot/sdk-go/api"
	"github.com/air-iot/sdk-go/conn/mqtt"
	"github.com/air-iot/sdk-go/conn/rabbit"
	"github.com/air-iot/sdk-go/conn/websocket"
	"github.com/air-iot/sdk-go/driver/convert"
	"github.com/air-iot/sdk-go/driver/entity"
	"github.com/air-iot/sdk-go/logger"
)

type App interface {
	Start(driver Driver, handlers ...Handler)
	WritePoints(point Point) error
	WriteEvent(event Event) error
	RunLog(l Log) error
	UpdateNode(id string, custom map[string]interface{}) error
	LogDebug(uid string, msg interface{})
	LogInfo(uid string, msg interface{})
	LogWarn(uid string, msg interface{})
	LogError(uid string, msg interface{})
	GetLogger() *logrus.Logger
	ApiClient() api.Client
	GetProjectId() string
	// ConvertValue(tag, raw interface{}) (map[string]interface{}, interface{}, error)
}

type Driver interface {
	Start(App, []byte) error
	Reload(App, []byte) error
	Run(app App, cmd *Command) (interface{}, error)
	BatchRun(app App, cmd *BatchCommand) (interface{}, error)
	WriteTag(app App, cmd *Command) (interface{}, error)
	Debug(App, []byte) (interface{}, error)
	Stop(App) error
	Schema(App) (string, error)
	GetVersion() string
}

type Handler interface {
	Start()
	Stop()
}

const (
	String  = "string"
	Float   = "float"
	Integer = "integer"
	Boolean = "boolean"
)

// app 数据采集类
type app struct {
	*logrus.Logger
	sendMethod   string
	mqtt         *mqtt.Mqtt
	rabbit       *rabbit.Amqp
	ws           *websocket.Conn
	api          api.Client
	driverId     string
	serviceId    string
	driverName   string
	distributed  string
	projectID    string
	host         string
	port         int
	stopped      bool
	healthTime   int
	intervalTime int
	cacheValue   sync.Map
	Version      string
}

// Point 存储数据
type Point struct {
	ID         string  `json:"id"`         // 设备编号
	CID        string  `json:"cid"`        // 子设备编号
	Fields     []Field `json:"fields"`     // 数据点
	UnixTime   int64   `json:"time"`       // 数据采集时间 毫秒数
	OnlineType string  `json:"onlineType"` // 资产在线状态类型
	//
	FieldTypes map[string]string `json:"fieldTypes"` // 数据点类型
}

type Event struct {
	ID       string      `json:"id"`      // 设备编号
	EventID  string      `json:"eventId"` // 事件ID
	UnixTime int64       `json:"time"`    // 数据采集时间 毫秒数
	Data     interface{} `json:"data"`    // 事件数据
}

type Log struct {
	SerialNo string `json:"serialNo"` // 流水号
	Status   string `json:"status"`   // 日志状态
	UnixTime int64  `json:"time"`     // 日志时间毫秒数
	Desc     string `json:"desc"`     // 描述
}

// Field 字段
type Field struct {
	Tag   interface{} `json:"tag"`   // 数据点
	Value interface{} `json:"value"` // 数据采集值
}

// MQTT mqtt配置参数
type MQTT struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
}

type pointTmp struct {
	ID         string                 `json:"id"`
	CID        string                 `json:"cid"`    // 子设备编号
	Source     string                 `json:"source"` // 标识设备数据
	Fields     map[string]interface{} `json:"fields"`
	UnixTime   int64                  `json:"time"`
	OnlineType string                 `json:"onlineType"` // 资产在线状态类型
	FieldTypes map[string]string      `json:"fieldTypes"` // 数据点类型
}

type wsRequest struct {
	RequestId string      `json:"requestId"`
	Action    string      `json:"action"`
	Data      interface{} `json:"data"`
}

type wsResponse struct {
	RequestId string `json:"requestId"`
	Data      result `json:"data"`
}

type result struct {
	Code   int         `json:"code"`
	Result interface{} `json:"result"`
}

type Command struct {
	NodeId   string      `json:"nodeId"`
	SerialNo string      `json:"serialNo"`
	Command  interface{} `json:"command"`
}

type BatchCommand struct {
	NodeIds  []string    `json:"nodeIds"`
	SerialNo string      `json:"serialNo"`
	Command  interface{} `json:"command"`
}

type resultMsg struct {
	Message string `json:"message"`
}

func init() {
	// 设置随机数种子
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())

	pflag.String("project", "default", "项目id")
	cfgPath := pflag.String("config", "./etc/", "配置文件")
	pflag.Parse()

	viper.SetDefault("log.level", "INFO")
	viper.SetDefault("host", "traefik")
	viper.SetDefault("port", 80)

	viper.SetDefault("mqtt.host", "mqtt")
	viper.SetDefault("mqtt.port", 1883)
	viper.SetDefault("mqtt.username", "admin")
	viper.SetDefault("mqtt.password", "public")

	viper.SetDefault("rabbit.host", "rabbit")
	viper.SetDefault("rabbit.port", 5672)
	viper.SetDefault("rabbit.username", "admin")
	viper.SetDefault("rabbit.password", "public")

	viper.SetDefault("ws.health", 30)
	viper.SetDefault("ws.interval", 10)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(*cfgPath)

	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		logrus.Fatalln("读取命令行参数错误,", err.Error())
	}

	if err := viper.ReadInConfig(); err != nil {
		logrus.Fatalln("读取配置,", err.Error())
	}
}

// NewApp 创建App
func NewApp() App {
	var (
		host        = viper.GetString("host")
		port        = viper.GetInt("port")
		ak          = viper.GetString("credentials.ak")
		sk          = viper.GetString("credentials.sk")
		driverId    = viper.GetString("driver.id")
		driverName  = viper.GetString("driver.name")
		distributed = viper.GetString("driver.distributed")
		sendMethod  = viper.GetString("driver.sendMethod")
		logLevel    = viper.GetString("log.level")
		projectID   = viper.GetString("project")
		health      = viper.GetInt("ws.health")
		interval    = viper.GetInt("ws.interval")
	)
	if driverId == "" || driverName == "" {
		panic("驱动id或name不能为空")
	}
	if projectID == "" {
		projectID = "default"
	}
	a := new(app)
	a.Logger = logger.NewLogger(logLevel)
	a.distributed = distributed
	a.driverId = driverId
	a.driverName = driverName
	a.sendMethod = sendMethod
	a.serviceId = uuid.New().String()
	a.projectID = projectID
	logrus.Infof("项目ID: %s", projectID)

	mqttConf := new(MQTT)
	mqttConf.Host = viper.GetString("mqtt.host")
	mqttConf.Port = viper.GetInt("mqtt.port")
	mqttConf.Username = viper.GetString("mqtt.username")
	mqttConf.Password = viper.GetString("mqtt.password")

	logrus.Infof("项目ID[%s]处理前配置: %+v", projectID, mqttConf)
	DecryptConfig(mqttConf)
	logrus.Infof("项目ID[%s]处理后配置: %+v", projectID, mqttConf)

	mqttCli, err := mqtt.NewMqtt(mqttConf.Host, mqttConf.Port, mqttConf.Username, mqttConf.Password)
	if err != nil {
		logrus.Fatalln("连接mqtt错误,", err.Error())
	}
	a.mqtt = mqttCli
	if sendMethod == "rabbit" {
		rabbitCli, err := rabbit.NewAmqp(viper.GetString("rabbit.host"), viper.GetInt("rabbit.port"), ak, sk, "/")
		if err != nil {
			//panic(err)
			logrus.Fatalln("连接rabbit错误,", err.Error())
		}
		a.rabbit = rabbitCli
	}
	a.host = host
	a.port = port
	a.stopped = false
	a.api = api.NewClient("http", host, port, projectID, ak, sk)
	if health == 0 {
		health = 30
	}
	if interval == 0 {
		interval = 10
	}
	a.healthTime = health
	a.intervalTime = interval
	a.cacheValue = sync.Map{}
	_, pluginFilePath, _, _ := runtime.Caller(0)
	configDir := filepath.Dir(pluginFilePath)
	if parts := strings.Split(configDir, "@"); len(parts) > 1 {
		a.Version = strings.TrimSuffix(strings.TrimPrefix(parts[len(parts)-1], "v"), "/driver")
		a.Version = strings.TrimSuffix(a.Version, "\\driver")
	} else {
		a.Version = pluginFilePath
	}
	return a
}

func DecryptConfig(c interface{}) {

	var cfg map[string]interface{}
	err := json.CopyByJson(&cfg, c)
	if err != nil {
		log.Fatalln("struct to map fail: ", err.Error())
	}

	for k, v := range cfg {
		cfg[k] = decryptConfig(v)
	}

	err = json.CopyByJson(c, &cfg)
	if err != nil {
		log.Fatalln("map to struct fail: ", err.Error())
	}
}

func decryptConfig(old interface{}) interface{} {
	switch val := old.(type) {
	case map[string]interface{}:
		for k, v := range val {
			val[k] = decryptConfig(v)
		}
		return val
	case *map[string]interface{}:
		for k, v := range *val {
			(*val)[k] = decryptConfig(v)
		}
		return val
	case []interface{}:
		for i, v := range val {
			val[i] = decryptConfig(v)
		}
		return val
	case *[]interface{}:
		for i, v := range *val {
			(*val)[i] = decryptConfig(v)
		}
		return val
	case []map[string]interface{}:
		var tmp = make([]interface{}, len(val))
		for i, v := range val {
			tmp[i] = decryptConfig(v)
		}
		return tmp
	case *[]map[string]interface{}:
		var tmp = make([]interface{}, len(*val))
		for i, v := range *val {
			tmp[i] = decryptConfig(v)
		}
		return tmp
	case string:
		if strings.HasPrefix(val, "ENC(") && strings.HasSuffix(val, ")") {
			vStr := strings.TrimSuffix(strings.TrimPrefix(val, "ENC("), ")")
			return cryptor.Base64StdDecode(vStr)
		} else if strings.HasPrefix(val, "AES(") && strings.HasSuffix(val, ")") {
			vStr := strings.TrimSuffix(strings.TrimPrefix(val, "AES("), ")")
			desVal, err := aaes.Decrypt(vStr, aaes.GetPKey())
			if err != nil {
				panic(fmt.Sprintf("解析密码错误,%v", err))
			}
			return string(desVal)
		}
		return val
	default:
		return val
	}
}

// Start 开始服务
func (p *app) Start(driver Driver, handlers ...Handler) {
	p.stopped = false
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	for _, handler := range handlers {
		handler.Start()
	}
	var wsConnected = false
	var reloadFlag = false

	go func() {
		var timeConnect = 0
		var timeOut = 10
		for {
			if p.stopped {
				return
			}
			var err error
			connMap, _ := json.Marshal(map[string]string{
				"driverId":    p.driverId,
				"driverName":  p.driverName,
				"serviceId":   p.serviceId,
				"distributed": p.distributed,
				"projectId":   p.projectID,
			})
			p.ws, err = websocket.DialWS(fmt.Sprintf(`ws://%s:%d/driver/ws?connInfo=%s&format=hex`, p.host, p.port, hex.EncodeToString(connMap)))
			if err != nil {
				timeConnect++
				if timeConnect > 5 {
					timeOut = 60
				}
				logrus.Errorf("尝试重新连接WebSocket第 %d 次失败,%s", timeConnect, err.Error())
				time.Sleep(time.Second * time.Duration(timeOut))
				continue
			}

			ts := time.Now().Local()
			p.ws.SetPongHandler(func(appData string) error {
				logrus.Infof("pong 值 %v", appData)
				ids := strings.Split(appData, ":")
				if len(ids) != 2 {
					logrus.Infoln("pong 值长度错误")
					return fmt.Errorf("pong 值长度错误")
				}
				if ids[1] != p.serviceId {
					logrus.Infoln("pong 返回服务id错误")
					return fmt.Errorf("pong 返回服务id错误")
				}
				ts = time.Now().Local()
				return nil
			})
			ctx, cancel := context.WithCancel(context.Background())
			go func() {
				for {
					select {
					case <-ctx.Done():
						logrus.Infoln("关闭心跳检查")
						return
					default:
						logrus.Infoln("心跳检查")
						if err := p.ws.WriteMessage(gws.PingMessage, []byte(p.serviceId)); err != nil {
							logrus.Errorf("心跳检查错误,%s", err.Error())
							p.ws.Close()
							time.Sleep(time.Second * time.Duration(p.healthTime))
							return
						}
					}
					time.Sleep(time.Second * time.Duration(p.healthTime))
				}
			}()
			go func() {
				for {
					select {
					case <-ctx.Done():
						logrus.Infoln("关闭检查周期")
						return
					default:
						logrus.Infof("心跳检查上次更新时间 %v", ts.String())
						if ts.Add(time.Second * time.Duration(p.healthTime) * 3).After(time.Now().Local()) {
							logrus.Infof("健康检查时间 %v 正常", ts.String())
						} else {
							logrus.Errorln("心跳检查时间超时,关闭连接")
							p.ws.Close()
							time.Sleep(time.Second * time.Duration(p.intervalTime))
							return
						}
					}
					time.Sleep(time.Second * time.Duration(p.intervalTime))
				}
			}()

			var handler = func() {
				for {
					var msg1 = new(wsRequest)
					err := p.ws.ReadJSON(&msg1)
					if err != nil {
						p.Logger.Warnf("读数据错误: %s", err.Error())
						return
					}

					var r result
					switch msg1.Action {
					case "start":
						reloadFlag = true
						c, err := p.api.DriverConfig(p.projectID, p.driverId, p.serviceId)
						if err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("查询配置错误,%s", err.Error())}}
						} else {
							p.cacheValue = sync.Map{}
							if err := driver.Start(p, c); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								r = result{Code: http.StatusOK, Result: resultMsg{Message: "驱动启动成功"}}
							}
						}
					case "reload":
						reloadFlag = true
						c, err := p.api.DriverConfig(p.projectID, p.driverId, p.serviceId)
						if err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("查询配置错误,%s", err.Error())}}
						} else {
							p.cacheValue = sync.Map{}
							if err := driver.Reload(p, c); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								r = result{Code: http.StatusOK, Result: resultMsg{Message: "驱动重启成功"}}
							}
						}
					case "run":
						cmdByte, _ := json.Marshal(msg1.Data)
						cmd := new(Command)
						err := json.Unmarshal(cmdByte, cmd)
						if err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("指令转换错误,%s", err.Error())}}
						} else {
							//cmdByte, _ := json.Marshal(cmd.Command)
							if res1, err := driver.Run(p, cmd); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								if res1 == nil {
									res1 = resultMsg{"指令写入成功"}
								}
								r = result{Code: http.StatusOK, Result: res1}
							}
						}
					case "batchRun":
						cmdByte, _ := json.Marshal(msg1.Data)
						cmd := new(BatchCommand)
						err := json.Unmarshal(cmdByte, cmd)
						if err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("指令转换错误,%s", err.Error())}}
						} else {
							//cmdByte, _ := json.Marshal(cmd.Command)
							if res1, err := driver.BatchRun(p, cmd); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								if res1 == nil {
									res1 = resultMsg{"指令写入成功"}
								}
								r = result{Code: http.StatusOK, Result: res1}
							}
						}
					case "writeTag":
						cmdByte, _ := json.Marshal(msg1.Data)
						cmd := new(Command)
						err := json.Unmarshal(cmdByte, cmd)
						if err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("数据点转换错误,%s", err.Error())}}
						} else {
							//cmdByte, _ := json.Marshal(cmd.Command)
							if res1, err := driver.WriteTag(p, cmd); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								if res1 == nil {
									res1 = resultMsg{"数据点写入成功"}
								}
								r = result{Code: http.StatusOK, Result: res1}
							}
						}
					case "debug":
						debugByte, ok := msg1.Data.(string)
						if !ok {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: fmt.Sprintf("数据非字符串")}}
						} else {
							if r1, err := driver.Debug(p, []byte(debugByte)); err != nil {
								r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
							} else {
								r = result{Code: http.StatusOK, Result: r1}
							}
						}
					case "schema":
						if r1, err := driver.Schema(p); err != nil {
							r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: err.Error()}}
						} else {
							r1 = strings.ReplaceAll(r1, "__version__", driver.GetVersion())
							r1 = strings.ReplaceAll(r1, "__sdk_version__", p.Version)
							r = result{Code: http.StatusOK, Result: r1}
						}
					default:
						r = result{Code: http.StatusBadRequest, Result: resultMsg{Message: "未找到执行动作"}}
					}
					err = p.ws.WriteJSON(&wsResponse{RequestId: msg1.RequestId, Data: r})
					if err != nil {
						p.Logger.Warnln("写数据错误,", err.Error())
						return
					}
				}
			}
			timeConnect = 0
			timeOut = 10
			wsConnected = true
			handler()
			cancel()
		}
	}()
	//if p.distributed == "" {
	go func() {
		var c1 = make([]byte, 0)
		for {
			if wsConnected {
				c, err := p.api.DriverConfig(p.projectID, p.driverId, p.serviceId)
				if err != nil {
					p.Logger.Errorln("查询配置错误,", err.Error())
					time.Sleep(time.Second * 60)
					continue
				} else if string(c) == "[]" {
					if reloadFlag {
						break
					}
					p.Logger.Warnln("查询配置为空")
					time.Sleep(time.Second * 60)
					continue
				} else {
					c1 = c
				}
				break
			} else {
				time.Sleep(time.Second * 10)
			}
		}
		if !reloadFlag && string(c1) != "[]" {
			if err := driver.Start(p, c1); err != nil {
				p.Logger.Warnln("驱动启动错误,", err.Error())
			}
		}
	}()
	//}
	sig := <-ch
	close(ch)
	if err := driver.Stop(p); err != nil {
		p.Logger.Warnln("驱动停止,", err.Error())
	}
	p.stop()
	for _, handler := range handlers {
		handler.Stop()
	}
	p.Logger.Debugln("关闭服务,", sig)
	os.Exit(0)
}

// GetLogger 获取日志
func (p *app) GetLogger() *logrus.Logger {
	return p.Logger
}

func (p *app) GetProjectId() string {
	return p.projectID
}

// Stop 服务停止
func (p *app) stop() {
	p.stopped = true
	if p.ws != nil {
		p.ws.Close()
	}
	p.mqtt.Close()
	if p.sendMethod == "rabbit" {
		p.rabbit.Close()
	}
}

// SendMsg 发送消息
func (p *app) send(topic string, b []byte) error {
	if token := p.mqtt.Publish(topic, 0, false, string(b)); token.Wait() && token.Error() != nil {
		return token.Error()
	}
	return nil
}

// WritePoints 写数据点数据
func (p *app) WritePoints(point Point) error {
	if point.ID == "" || point.Fields == nil || len(point.Fields) == 0 {
		return errors.New("数据有空值")
	}
	fields := make(map[string]interface{})
	for _, field := range point.Fields {
		if field.Tag == nil || field.Value == nil {
			p.Logger.Warnf("资产 [%s] 数据点为空", point.ID)
			continue
		}
		tagByte, err := json.Marshal(field.Tag)
		if err != nil {
			p.Logger.Warnf("资产 [%s] 数据点序列化错误: %s", point.ID, err.Error())
			continue
		}

		tag := new(entity.Tag)
		err = json.Unmarshal(tagByte, tag)
		if err != nil {
			p.Logger.Warnf("资产 [%s] 数据点序列化tag结构体错误: %s", point.ID, err.Error())
			continue
		}
		var value decimal.Decimal
		//vType := reflect.TypeOf(raw).String()
		switch valueTmp := field.Value.(type) {
		case float32:
			value = decimal.NewFromFloat32(valueTmp)
		case float64:
			if math.IsNaN(valueTmp) || math.IsInf(valueTmp, 0) {
				p.Logger.Warnf("资产 [%s] 设备数据点 %s 值不合法: %s", point.ID, tag.ID)
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
		default:
			fields[tag.ID] = field.Value
			continue
		}
		val := convert.ConvertValue(tag, value)
		cacheKey := fmt.Sprintf("%s__%s", point.ID, tag.ID)
		preValF, ok := p.cacheValue.Load(cacheKey)
		var preVal *decimal.Decimal
		if ok {
			preF, ok := preValF.(*float64)
			if ok && preF != nil {
				preValue := decimal.NewFromFloat(*preF)
				preVal = &preValue
			}
		}
		newVal, rawVal, invalidType, save := convert.ConvertRange(tag.Range, preVal, &val)
		if newVal != nil {
			fields[tag.ID] = newVal
			if save {
				p.cacheValue.Store(cacheKey, newVal)
			}
		}
		if rawVal != nil {
			fields[fmt.Sprintf("%s__invalid", tag.ID)] = rawVal
		}
		if invalidType != "" {
			fields[fmt.Sprintf("%s__invalid__type", tag.ID)] = invalidType
		}
		//val := p.convertRange(tag.Range, p.convertValue(tag, value))
		//if val != nil {
		//	if fieldType, ok := point.FieldTypes[tag.ID]; ok {
		//		switch fieldType {
		//		case Integer:
		//			fields[tag.ID] = int(*val)
		//		default:
		//			fields[tag.ID] = val
		//		}
		//	} else {
		//		fields[tag.ID] = val
		//	}
		//}
	}

	if len(fields) == 0 {
		return errors.New("数据点为空值")
	}
	if point.UnixTime == 0 {
		point.UnixTime = time.Now().Local().UnixNano() / 1e6
	}
	b, err := json.Marshal(&pointTmp{ID: point.ID, CID: point.CID, Source: "device", UnixTime: point.UnixTime, Fields: fields, OnlineType: point.OnlineType, FieldTypes: point.FieldTypes})
	if err != nil {
		return err
	}
	p.Logger.Debugf("保存数据,%s", string(b))
	if p.sendMethod == "rabbit" {
		return p.rabbit.Send("data", fmt.Sprintf("data.%s.%s", p.projectID, point.ID), b)
	} else {
		return p.mqtt.Send(fmt.Sprintf("data/%s/%s", p.projectID, point.ID), string(b))
	}
}

func (p *app) WriteEvent(event Event) error {
	if event.ID == "" || event.EventID == "" {
		return errors.New("资产或事件ID为空")
	}
	b, err := json.Marshal(event)
	if err != nil {
		return err
	}
	if p.sendMethod == "rabbit" {
		return p.rabbit.Send("driverEvent", fmt.Sprintf("driverEvent.%s.%s", p.projectID, event.ID), b)
	} else {
		return p.mqtt.Send(fmt.Sprintf("driverEvent/%s/%s", p.projectID, event.ID), string(b))
	}
}

func (p *app) RunLog(l Log) error {
	if l.SerialNo == "" {
		return errors.New("流水号为空")
	}
	b, err := json.Marshal(l)
	if err != nil {
		return err
	}
	if p.sendMethod == "rabbit" {
		return p.rabbit.Send("driverRunLog", fmt.Sprintf("driverRunLog.%s", p.projectID), b)
	} else {
		return p.mqtt.Send(fmt.Sprintf("driverRunLog/%s", p.projectID), string(b))
	}
}

func (p *app) UpdateNode(id string, custom map[string]interface{}) error {
	data := make(map[string]interface{})
	for k, v := range custom {
		data[fmt.Sprintf("custom.%s", k)] = v
	}
	if err := p.api.UpdateNodeById(id, data, &map[string]interface{}{}); err != nil {
		return err
	}
	return nil
}

//// active fixed  boundary  discard
//func (p *app) convertRange(tagRange *Range, raw decimal.Decimal) (val *float64) {
//	value, _ := raw.Float64()
//	if tagRange == nil {
//		return &value
//	}
//	if tagRange.MinValue == nil || tagRange.MaxValue == nil || tagRange.Active == nil {
//		return &value
//	}
//	minValue := decimal.NewFromFloat(*tagRange.MinValue)
//	maxValue := decimal.NewFromFloat(*tagRange.MaxValue)
//
//	if raw.GreaterThanOrEqual(minValue) && raw.LessThanOrEqual(maxValue) {
//		return &value
//	}
//
//	switch *tagRange.Active {
//	case "fixed":
//		if tagRange.FixedValue == nil {
//			return &value
//		}
//		return tagRange.FixedValue
//	case "boundary":
//		if raw.LessThan(minValue) {
//			return tagRange.MinValue
//		}
//		if raw.GreaterThan(maxValue) {
//			return tagRange.MaxValue
//		}
//	case "discard":
//		return nil
//	default:
//		return &value
//	}
//	return &value
//}
//
//// ConvertValue 数据点值转换
//func (p *app) convertValue(tagTemp *Tag, raw decimal.Decimal) (val decimal.Decimal) {
//	var value = raw
//	if tagTemp.TagValue != nil {
//		if tagTemp.TagValue.MinRaw != nil {
//			minRaw := decimal.NewFromFloat(*tagTemp.TagValue.MinRaw)
//			if value.LessThan(minRaw) {
//				value = minRaw
//			}
//		}
//
//		if tagTemp.TagValue.MaxRaw != nil {
//			maxRaw := decimal.NewFromFloat(*tagTemp.TagValue.MaxRaw)
//			if value.GreaterThan(maxRaw) {
//				value = maxRaw
//			}
//		}
//
//		if tagTemp.TagValue.MinRaw != nil && tagTemp.TagValue.MaxRaw != nil && tagTemp.TagValue.MinValue != nil && tagTemp.TagValue.MaxValue != nil {
//			//value = (((rawTmp - minRaw) / (maxRaw - minRaw)) * (maxValue - minValue)) + minValue
//			minRaw := decimal.NewFromFloat(*tagTemp.TagValue.MinRaw)
//			maxRaw := decimal.NewFromFloat(*tagTemp.TagValue.MaxRaw)
//			minValue := decimal.NewFromFloat(*tagTemp.TagValue.MinValue)
//			maxValue := decimal.NewFromFloat(*tagTemp.TagValue.MaxValue)
//			if !maxRaw.Equal(minRaw) {
//				value = raw.Sub(minRaw).Div(maxRaw.Sub(minRaw)).Mul(maxValue.Sub(minValue)).Add(minValue)
//			}
//		}
//	}
//
//	if tagTemp.Fixed != nil {
//		value = value.Round(*tagTemp.Fixed)
//	}
//
//	if tagTemp.Mod != nil {
//		value = value.Mul(decimal.NewFromFloat(*tagTemp.Mod))
//	}
//
//	return value
//}

// Log 写日志数据
func (p *app) Log(topic string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	err = p.send("logs/"+topic, b)
	if err != nil {
		return
	}
}

// LogDebug 写日志数据
func (p *app) LogDebug(uid string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "uid": uid, "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	err = p.send(fmt.Sprintf("logs/%s/debug/%s", p.projectID, uid), b)
	if err != nil {
		return
	}
}

// LogInfo 写日志数据
func (p *app) LogInfo(uid string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "uid": uid, "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	err = p.send(fmt.Sprintf("logs/%s/info/%s", p.projectID, uid), b)
	if err != nil {
		return
	}
}

// LogWarn 写日志数据
func (p *app) LogWarn(uid string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "uid": uid, "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	err = p.send(fmt.Sprintf("logs/%s/warn/%s", p.projectID, uid), b)
	if err != nil {
		return
	}
}

// LogError 写日志数据
func (p *app) LogError(uid string, msg interface{}) {
	l := map[string]interface{}{"time": time.Now().Format("2006-01-02 15:04:05"), "uid": uid, "message": msg}
	b, err := json.Marshal(l)
	if err != nil {
		return
	}
	err = p.send(fmt.Sprintf("logs/%s/error/%s", p.projectID, uid), b)
	if err != nil {
		return
	}
}

// ApiClient api 接口客户端
func (p *app) ApiClient() api.Client {
	return p.api
}
