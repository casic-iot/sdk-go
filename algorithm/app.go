package algorithm

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"os/signal"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/air-iot/logger"
	"github.com/google/uuid"
	"github.com/sirupsen/logrus"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type App interface {
	Start(Service)
	GetLogger() *logrus.Logger
}

type Service interface {
	// Schema
	// @description 查询schema
	// @return result "算法配置schema,返回字符串"
	Schema(App) (result string, err error)

	// Start
	// @description 启动算法服务
	Start(App) error

	// Run
	// @description 执行算法服务
	// @param bts 执行参数 {"function":"算法名","input":{}} input 算法执行参数,应与输出的schema格式相同
	// @return result "自定义返回的格式,应与输出的schema格式相同"
	Run(app App, bts []byte) (result interface{}, err error)

	// Stop
	// @description 停止算法服务
	Stop(App) error
}

//const (
//	String  = "string"
//	Float   = "float"
//	Integer = "integer"
//	Boolean = "boolean"
//)

// app 数据采集类
type app struct {
	*logrus.Logger
	//mq      mq.MQ
	stopped bool
	cli     *Client
	clean   func()

	cacheValue sync.Map
}

func init() {
	// 设置随机数种子
	rand.Seed(time.Now().Unix())
	runtime.GOMAXPROCS(runtime.NumCPU())
	pflag.String("serviceId", "", "服务id")
	cfgPath := pflag.String("config", "./etc/", "配置文件")
	pflag.Parse()
	viper.SetDefault("log.level", 4)
	viper.SetDefault("log.format", "text")
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("algorithmGrpc.host", "algorithm")
	viper.SetDefault("algorithmGrpc.port", 9236)
	viper.SetDefault("algorithmGrpc.health.requestTime", 10)
	viper.SetDefault("algorithmGrpc.waitTime", 5)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	viper.AddConfigPath(*cfgPath)
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalln("读取命令行参数错误,", err.Error())
	}
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalln("读取配置错误,", err.Error())
	}
	if err := viper.Unmarshal(C); err != nil {
		log.Fatalln("配置解析错误: ", err.Error())
	}
}

// NewApp 创建App
func NewApp() App {
	a := new(app)
	if _, err := logger.NewLogger(C.Log); err != nil {
		panic(fmt.Errorf("初始化日志错误,%s", err))
	}

	if C.Algorithm.ID == "" || C.Algorithm.Name == "" {
		panic("算法id或name不能为空")
	}
	if C.ServiceID == "" {
		C.ServiceID = fmt.Sprintf("%s_%s", C.Algorithm.ID, uuid.New().String())
	}
	if C.AlgorithmGrpc.Health.RequestTime == 0 {
		C.AlgorithmGrpc.Health.RequestTime = 10
	}
	if C.AlgorithmGrpc.Health.Retry == 0 {
		C.AlgorithmGrpc.Health.Retry = 3
	}
	if C.AlgorithmGrpc.WaitTime == 0 {
		C.AlgorithmGrpc.WaitTime = 5
	}
	logger.Debugf("配置: %+v", *C)

	a.cacheValue = sync.Map{}
	return a
}

// Start 开始算法服务
func (a *app) Start(service Service) {
	a.stopped = false
	cli := Client{cacheConfig: sync.Map{}, cacheConfigNum: sync.Map{}}
	// grpc客户端Start
	a.cli = cli.Start(a, service)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	sig := <-ch
	close(ch)
	if err := service.Stop(a); err != nil {
		logger.Warnln("算法停止,", err.Error())
	}
	cli.Stop()
	a.stop()
	logger.Debugln("关闭算法,", sig)
	os.Exit(0)
}

// GetLogger 获取日志
func (a *app) GetLogger() *logrus.Logger {
	return a.Logger
}

// Stop 服务停止
func (a *app) stop() {
	a.stopped = true
	if a.clean != nil {
		a.clean()
	}
}
