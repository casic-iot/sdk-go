package data_relay

import (
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/air-iot/logger"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type App interface {
	Start(ext DataRelay)
	GetProjectId() string
}

// app 数据采集类
type app struct {
	stopped bool
	cli     *Client
	clean   func()
}

func init() {
	// 设置随机数种子
	runtime.GOMAXPROCS(runtime.NumCPU())
	pflag.String("project", "default", "项目id")
	pflag.String("serviceId", "", "服务id")
	cfgPath := pflag.String("config", "./etc/", "配置文件")
	pflag.Parse()
	viper.SetDefault("log.level", 4)
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("dataRelayGrpc.host", "data-relay")
	viper.SetDefault("dataRelayGrpc.port", 9232)
	viper.SetDefault("dataRelayGrpc.health.requestTime", "10s")
	viper.SetDefault("dataRelayGrpc.health.retry", 3)
	viper.SetDefault("dataRelayGrpc.waitTime", "5s")
	viper.SetDefault("dataRelayGrpc.timeout", "600s")
	viper.SetDefault("dataRelayGrpc.limit", 100)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	log.Println("配置文件路径", *cfgPath)
	viper.AddConfigPath(*cfgPath)
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		log.Fatalln("读取命令行参数错误,", err.Error())
	}
	if err := viper.ReadInConfig(); err != nil {
		log.Fatalln("读取配置错误,", err.Error())
	}
	if err := viper.Unmarshal(Cfg); err != nil {
		log.Fatalln("配置解析错误: ", err.Error())
	}
}

// NewApp 创建App
func NewApp() App {
	a := new(app)
	if Cfg.Service.ID == "" || Cfg.Service.Name == "" {
		panic("服务 id 和 name 不能为空")
	}
	Cfg.Log.Syslog.ServiceName = Cfg.Service.ID
	logger.InitLogger(Cfg.Log)
	logger.Debugf("配置=%+v", *Cfg)
	a.clean = func() {}
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

func (a *app) GetProjectId() string {
	return Cfg.Project
}

// Start 开始服务
func (a *app) Start(ext DataRelay) {
	a.stopped = false
	cli := Client{}
	a.cli = cli.Start(a, ext)
	ch := make(chan os.Signal, 1)
	signal.Notify(ch, syscall.SIGTERM, syscall.SIGINT, syscall.SIGKILL)
	sig := <-ch
	close(ch)
	cli.Stop()
	logger.Debugf("关闭服务: 信号=%v", sig)
	os.Exit(0)
}
