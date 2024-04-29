package etcd

import (
	"context"
	"fmt"

	"github.com/air-iot/logger"
	cfg "github.com/go-kratos/kratos/contrib/config/etcd/v2"
	"github.com/go-kratos/kratos/v2/config"
	"github.com/spf13/viper"
)

func ScanEtcd(etcdKey string, etcdCfg Config, conf interface{}) error {
	if len(etcdCfg.Endpoints) == 0 {
		return fmt.Errorf("etcd连接为空")
	}
	etcdClient, err := NewConn(etcdCfg)
	if err != nil {
		return fmt.Errorf("读配置,etcd连接创建错误: %w", err)
	}
	defer func() {
		if err := etcdClient.Close(); err != nil {
			logger.WithContext(logger.NewErrorContext(context.Background(), err)).Errorf("读配置,关闭etcd连接错误")
		}
	}()
	etcdSource, err := cfg.New(etcdClient, cfg.WithPath(etcdKey), cfg.WithPrefix(true))
	if err != nil {
		return fmt.Errorf("读etcd配置错误: %w", err)
	}
	// create a config instance with source
	c2 := config.New(config.WithSource(
		etcdSource),
	//file.NewSource(fpath),
	//env.NewSource("")),
	//config.WithResolver(func(m map[string]interface{}) error {
	//	DecryptConfig(m)
	//	return nil
	//}),
	)
	defer func() {
		if err := c2.Close(); err != nil {
			logger.WithContext(logger.NewErrorContext(context.Background(), err)).Infof("关闭etcd配置源错误")
		}
	}()
	if err := c2.Load(); err != nil {
		return fmt.Errorf("load etcd配置错误: %w", err)
	}
	var c2m map[string]interface{}
	if err := c2.Scan(&c2m); err != nil {
		return fmt.Errorf("解析etcd配置到map错误: %w", err)
	}
	if err := viper.MergeConfigMap(c2m); err != nil {
		return fmt.Errorf("viper合并etcd配置错误: %w", err)
	}
	if err := viper.Unmarshal(conf); err != nil {
		return fmt.Errorf("viper解析etcd配置到结构体错误: %w", err)
	}
	return nil
}
