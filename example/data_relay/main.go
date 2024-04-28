package main

import (
	"context"
	"github.com/air-iot/sdk-go/v4/data_relay"
	"github.com/air-iot/sdk-go/v4/example/data_relay/app"
)

func main() {
	d := new(app.TestRelay)
	d.Ctx, d.Cancel = context.WithCancel(context.Background())
	data_relay.NewApp().Start(d)
}
