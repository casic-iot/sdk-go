package main

import (
	"context"
	"github.com/air-iot/sdk-go/v4/driver"
	"github.com/air-iot/sdk-go/v4/example/driver/app"
)

func main() {
	// 创建采集主程序
	d := new(app.TestDriver)
	d.Ctx, d.Cancel = context.WithCancel(context.Background())
	driver.NewApp().Start(d)
}
