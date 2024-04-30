package flow_extionsion

import (
	"fmt"
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
	Start(ext Extension)
}

// app 数据采集类
type app struct {
	stopped bool
	cli     *Client
	clean   func()
}

func Init() {
	// 设置随机数种子
	runtime.GOMAXPROCS(runtime.NumCPU())
	cfgPath := pflag.String("config", "./etc/", "配置文件")
	viper.SetDefault("log.level", 4)
	viper.SetDefault("log.format", "json")
	viper.SetDefault("log.output", "stdout")
	viper.SetDefault("flowEngine.host", "flow-engine")
	viper.SetDefault("flowEngine.port", 2333)
	viper.SetDefault("extension.timeout", 600)
	viper.SetConfigType("env")
	viper.AutomaticEnv()
	viper.SetConfigType("yaml")
	viper.SetConfigName("config")
	pflag.Parse()
	log.Println("配置文件路径", *cfgPath)
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
	if Cfg.Extension.Id == "" || Cfg.Extension.Name == "" {
		panic("流程扩展服务id和name不能为空")
	}
	Cfg.Log.Syslog.ServiceName = Cfg.Extension.Id
	logger.InitLogger(Cfg.Log)
	logger.Infof("启动配置=%+v", *Cfg)
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

// Start 开始服务
func (a *app) Start(ext Extension) {
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
