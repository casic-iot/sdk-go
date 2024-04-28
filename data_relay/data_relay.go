package data_relay

import (
	"context"
	"net/http"
)

type DataRelay interface {

	// Start
	// @description 启动服务
	// @param driverConfig "配置数据"
	Start(ctx context.Context, app App, config []byte) (err error)

	// HttpProxy
	// @description 代理接口
	// @param t 请求接口标识
	// @param header 请求头
	// @param data 请求数据
	// @return result "响应结果,自定义返回的格式"
	HttpProxy(ctx context.Context, app App, t string, header http.Header, data []byte) (result []byte, err error)
}
