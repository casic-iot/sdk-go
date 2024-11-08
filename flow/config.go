package flow

import (
	"context"
	"encoding/hex"
	"github.com/air-iot/logger"
	"google.golang.org/grpc/metadata"
)

// Cfg 全局配置(需要先执行MustLoad，否则拿不到配置)
var Cfg = new(Config)

type Config struct {
	Flow struct {
		Name    string   `json:"name" yaml:"name"`
		Mode    TaskMode `json:"mode" yaml:"mode"`
		Timeout uint     `json:"timeout" yaml:"timeout"`
	} `json:"flow" yaml:"flow"`
	FlowEngine Grpc          `json:"flowEngine" yaml:"flowEngine"`
	Log        logger.Config `json:"log" yaml:"log"`
	Pprof      struct {
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

type TaskMode string

const (
	UserTask    TaskMode = "user"
	ServiceTask TaskMode = "service"
)

func GetGrpcContext(ctx context.Context, name string, mode TaskMode) context.Context {
	md := metadata.New(map[string]string{
		"name": hex.EncodeToString([]byte(name)),
		"mode": hex.EncodeToString([]byte(mode))})
	// 发送 metadata
	// 创建带有meta的context
	return metadata.NewOutgoingContext(ctx, md)
}
