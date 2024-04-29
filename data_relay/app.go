package data_relay

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

	api_client_go "github.com/air-iot/api-client-go/v4"
	"github.com/air-iot/json"
	"github.com/air-iot/logger"
	"github.com/air-iot/sdk-go/v4/etcd"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type App interface {
	Start(ext DataRelay)
	GetProjectId() string
	GetAPIClient() *api_client_go.Client
}

// app 数据采集类
type app struct {
	stopped   bool
	cli       *Client
	etcdConn  *clientv3.Client
	apiClient *api_client_go.Client
	clean     func()
}

func Init() {
	// 设置随机数种子
	runtime.GOMAXPROCS(runtime.NumCPU())
	pflag.String("project", "default", "项目id")
	pflag.String("instanceId", "", "服务id")
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
	viper.SetDefault("dataRelayGrpc.limit", 100)
	viper.SetDefault("etcdConfig", "/airiot/config/dev.json")
	viper.SetDefault("etcd.endpoints", []string{"etcd:2379"})
	viper.SetDefault("etcd.username", "root")
	viper.SetDefault("etcd.password", "dell123")
	viper.SetDefault("etcd.dialTimeout", 60)
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
	Init()
	err := etcd.ScanEtcd(Cfg.EtcdConfig, Cfg.Etcd, Cfg)
	if err != nil {
		panic(fmt.Errorf("读etcd错误,%w", err))
	}
	var cfgMap map[string]interface{}
	if err := json.CopyByJson(&cfgMap, Cfg); err != nil {
		panic(fmt.Errorf("转配置为map错误,%w", err))
	}

	if err := viper.MergeConfigMap(cfgMap); err != nil {
		panic(fmt.Errorf("合并map配置错误,%w", err))
	}
	if err := viper.ReadInConfig(); err != nil {
		panic(fmt.Errorf("读取配置错误,%w", err))
	}
	if err := viper.Unmarshal(Cfg); err != nil {
		panic(fmt.Errorf("配置解析错误,%w", err))
	}
	a := new(app)
	if Cfg.Service.ID == "" || Cfg.Service.Name == "" {
		panic("服务 id 和 name 不能为空")
	}
	Cfg.Log.Syslog.ServiceName = Cfg.Service.ID
	logger.InitLogger(Cfg.Log)
	logger.Debugf("配置=%+v", *Cfg)

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
	conn, err := etcd.NewConn(Cfg.Etcd)
	if err != nil {
		panic(err)
	}
	a.etcdConn = conn
	apiCli, clean, err := api_client_go.NewClient(conn, Cfg.App.API)
	if err != nil {
		panic(err)
	}
	a.apiClient = apiCli
	a.clean = func() {
		clean()
		err := conn.Close()
		if err != nil {
			logger.Errorf("关闭etcd: %v", err)
		}
	}

	return a
}

func (a *app) GetProjectId() string {
	return Cfg.Project
}

func (a *app) GetAPIClient() *api_client_go.Client {
	return a.apiClient
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
	a.clean()
	os.Exit(0)
}
