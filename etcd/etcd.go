package etcd

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"

	"github.com/air-iot/logger"
)

type Config struct {
	Endpoints        []string      `json:"endpoints" yaml:"endpoints"`
	DialTimeout      int           `json:"dialTimeout" yaml:"dialTimeout"`
	Username         string        `json:"username" yaml:"username"`
	Password         string        `json:"password" yaml:"password"`
	AutoSyncInterval time.Duration `json:"autoSyncInterval" yaml:"autoSyncInterval"`
}

func New(cfg Config) (*clientv3.Client, func(), error) {
	cli, err := NewConn(cfg)
	if err != nil {
		return nil, nil, err
	}
	cleanFunc := func() {
		err := cli.Close()
		if err != nil {
			logger.WithContext(logger.NewErrorContext(context.Background(), err)).Errorf("关闭etcd错误")
		}
	}

	return cli, cleanFunc, nil
}

func NewConn(cfg Config) (*clientv3.Client, error) {
	client, err := clientv3.New(clientv3.Config{
		Endpoints:        cfg.Endpoints,
		DialTimeout:      time.Second * time.Duration(cfg.DialTimeout),
		DialOptions:      []grpc.DialOption{grpc.WithBlock()},
		Username:         cfg.Username,
		Password:         cfg.Password,
		AutoSyncInterval: cfg.AutoSyncInterval,
	})
	if err != nil {
		return nil, fmt.Errorf("创建etcd客户端错误: %w", err)
	}
	return client, nil
}
