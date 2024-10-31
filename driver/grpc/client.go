package grpc

import (
	"context"
	"encoding/hex"
	"time"

	"google.golang.org/grpc/metadata"
)

type Config struct {
	Host   string `json:"host" yaml:"host"`
	Port   int    `json:"port" yaml:"port"`
	Health struct {
		RequestTime time.Duration `json:"requestTime" yaml:"requestTime"`
		Retry       int           `json:"retry" yaml:"retry"`
	} `json:"health" yaml:"health"`
	Stream struct {
		Heartbeat time.Duration `json:"heartbeat" yaml:"heartbeat"`
	} `json:"stream" yaml:"stream"`
	WaitTime time.Duration `json:"waitTime" yaml:"waitTime"`
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`
	Limit    int           `json:"limit" yaml:"limit"`
}

func GetGrpcContext(ctx context.Context, serviceId, projectId, driverId, driverName, sessionId string) context.Context {
	md := metadata.New(map[string]string{
		"serviceId":  hex.EncodeToString([]byte(serviceId)),
		"projectId":  hex.EncodeToString([]byte(projectId)),
		"driverId":   hex.EncodeToString([]byte(driverId)),
		"driverName": hex.EncodeToString([]byte(driverName)),
		"sessionId":  hex.EncodeToString([]byte(sessionId)),
	})
	// 发送 metadata
	// 创建带有meta的context
	return metadata.NewOutgoingContext(ctx, md)
}
