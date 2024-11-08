package flow_extionsion

import (
	"context"
	"encoding/hex"

	"github.com/air-iot/logger"
	"google.golang.org/grpc/metadata"
)

// Cfg 全局配置(需要先执行MustLoad，否则拿不到配置)
var Cfg = new(Config)

type Config struct {
	Log        logger.Config `json:"log" yaml:"log"`
	FlowEngine Grpc          `json:"flowEngine" yaml:"flowEngine"`
	Extension  struct {
		Id      string `json:"id" yaml:"id"`
		Name    string `json:"name" yaml:"name"`
		Timeout uint   `json:"timeout" yaml:"timeout"`
	} `json:"extension" yaml:"extension"`
	Pprof struct {
		Enable bool   `json:"enable" yaml:"enable"`
		Host   string `json:"host" yaml:"host"`
		Port   string `json:"port" yaml:"port"`
	} `json:"pprof" yaml:"pprof"`
}

type Grpc struct {
	Host  string `json:"host" yaml:"host"`
	Port  int    `json:"port" yaml:"port"`
	Limit int    `json:"limit" yaml:"limit"`
}

func GetGrpcContext(ctx context.Context, id, name string) context.Context {
	md := metadata.New(map[string]string{
		"id":   hex.EncodeToString([]byte(id)),
		"name": hex.EncodeToString([]byte(name)),
	})
	// 发送 metadata
	// 创建带有meta的context
	return metadata.NewOutgoingContext(ctx, md)
}
