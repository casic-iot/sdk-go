package algorithm

import (
	"context"
	"encoding/hex"
	"github.com/air-iot/logger"
	"google.golang.org/grpc/metadata"
)

var Cfg = new(Config)

type Config struct {
	ServiceID string `json:"serviceId" yaml:"serviceId" mapstructure:"serviceId"`
	//Project   string `json:"project" yaml:"project" mapstructure:"project"`
	Algorithm struct {
		ID      string `json:"id" yaml:"id"`
		Name    string `json:"name" yaml:"name"`
		Timeout uint   `json:"timeout" yaml:"timeout"`
	} `json:"algorithm" yaml:"algorithm"`
	AlgorithmGrpc GrpcConfig    `json:"algorithmGrpc" yaml:"algorithmGrpc"`
	Log           logger.Config `json:"log" yaml:"log"`
	//MQ         mq.Config   `json:"mq" yaml:"mq"`
	Pprof struct {
		Enable bool   `json:"enable" yaml:"enable"`
		Host   string `json:"host" yaml:"host"`
		Port   string `json:"port" yaml:"port"`
	} `json:"pprof" yaml:"pprof"`
}

type GrpcConfig struct {
	Host   string `json:"host" yaml:"host"`
	Port   int    `json:"port" yaml:"port"`
	Health struct {
		RequestTime int `json:"requestTime" yaml:"requestTime"`
		Retry       int `json:"retry" yaml:"retry"`
	} `json:"health" yaml:"health"`
	WaitTime int `json:"waitTime" yaml:"waitTime"`
	Limit    int `json:"limit" yaml:"limit"`
}

func GetGrpcContext(ctx context.Context, serviceId, id, name string) context.Context {
	md := metadata.New(map[string]string{
		"serviceId":     hex.EncodeToString([]byte(serviceId)),
		"algorithmId":   hex.EncodeToString([]byte(id)),
		"algorithmName": hex.EncodeToString([]byte(name))})
	// 发送 metadata
	// 创建带有meta的context
	return metadata.NewOutgoingContext(ctx, md)
}
