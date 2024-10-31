package driver

import (
	"context"
	"net/http"

	"github.com/air-iot/sdk-go/v4/driver/entity"
)

type Driver interface {
	// Schema
	// @description 查询返回驱动配置schema内容
	// @param locale 国际化语言 zh
	// @return schema "驱动配置schema"
	Schema(ctx context.Context, app App, locale string) (schema string, err error)

	// Start
	// @description 驱动启动
	// @param driverConfig "包含实例、模型及设备数据"
	Start(ctx context.Context, app App, driverConfig []byte) (err error)

	// Run
	// @description 运行指令,向设备写入数据
	// @param command 指令参数{"table":"表标识","id":"设备编号","serialNo":"流水号","command":{}} command 指令内容
	// @return result "自定义返回的格式或者空"
	Run(ctx context.Context, app App, command *entity.Command) (result interface{}, err error)

	// BatchRun
	// @description 批量运行指令,向多设备写入数据
	// @param command 指令参数 {"table":"表标识", "ids": ["设备编号"], "serialNo": "流水号", 'command': {}}  command 指令内容
	// @return result "自定义返回的格式或者空"
	BatchRun(ctx context.Context, app App, command *entity.BatchCommand) (result interface{}, err error)

	// WriteTag
	// @description 数据点写入
	// @param command {"table":"表标识","id":"设备编号","serialNo":"流水号","command":{}} command 指令内容
	// @return result "自定义返回的格式或者空"
	WriteTag(ctx context.Context, app App, command *entity.Command) (result interface{}, err error)

	// Debug
	// @description 调试驱动
	// @param debugConfig object 调试参数
	// @return result "调试结果,自定义返回的格式"
	Debug(ctx context.Context, app App, debugConfig []byte) (result interface{}, err error)

	// HttpProxy
	// @description 代理接口
	// @param t 请求接口标识
	// @param header 请求头
	// @param data 请求数据
	// @return result "响应结果,自定义返回的格式"
	HttpProxy(ctx context.Context, app App, t string, header http.Header, data []byte) (result interface{}, err error)

	// Stop
	// @description 驱动停止处理
	Stop(ctx context.Context, app App) (err error)
}
