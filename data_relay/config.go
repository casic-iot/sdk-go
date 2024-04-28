package data_relay

import (
	"github.com/air-iot/logger"
	"github.com/air-iot/sdk-go/v4/data_relay/grpc"
)

// Cfg 全局配置(需要先执行MustLoad，否则拿不到配置)
var Cfg = new(Config)

type Config struct {
	Log       logger.Config `json:"log" yaml:"log"`
	ServiceID string        `json:"serviceId" yaml:"serviceId" mapstructure:"serviceId"`
	Project   string        `json:"project" yaml:"project" mapstructure:"project"`
	Service   struct {
		ID   string `json:"id" yaml:"id"`
		Name string `json:"name" yaml:"name"`
	} `json:"service" yaml:"service"`
	DataRelayGrpc grpc.Config `json:"dataRelayGrpc" yaml:"dataRelayGrpc"`
	Pprof         struct {
		Enable bool   `json:"enable" yaml:"enable"`
		Host   string `json:"host" yaml:"host"`
		Port   string `json:"port" yaml:"port"`
	} `json:"pprof" yaml:"pprof"`
}
