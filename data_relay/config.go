package data_relay

import (
	grpcConfig "github.com/air-iot/api-client-go/v4/config"
	"github.com/air-iot/logger"
	"github.com/air-iot/sdk-go/v4/data_relay/grpc"
	"github.com/air-iot/sdk-go/v4/etcd"
)

// Cfg 全局配置(需要先执行MustLoad，否则拿不到配置)
var Cfg = new(Config)

type Config struct {
	Log        logger.Config `json:"log" yaml:"log"`
	InstanceID string        `json:"instanceId" yaml:"instanceId" mapstructure:"instanceId"`
	Project    string        `json:"project" yaml:"project" mapstructure:"project"`
	Service    struct {
		ID   string `json:"id" yaml:"id"`
		Name string `json:"name" yaml:"name"`
	} `json:"service" yaml:"service"`
	DataRelayGrpc grpc.Config `json:"dataRelayGrpc" yaml:"dataRelayGrpc"`
	Pprof         struct {
		Enable bool   `json:"enable" yaml:"enable"`
		Host   string `json:"host" yaml:"host"`
		Port   string `json:"port" yaml:"port"`
	} `json:"pprof" yaml:"pprof"`
	EtcdConfig string      `json:"etcdConfig" yaml:"etcdConfig"`
	Etcd       etcd.Config `json:"etcd" yaml:"etcd"`
	App        struct {
		API grpcConfig.Config `json:"api" yaml:"API"`
	} `json:"app" yaml:"app"`
}
