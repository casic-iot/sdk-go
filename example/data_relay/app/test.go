package app

import (
	"context"
	"net/http"

	"github.com/air-iot/sdk-go/v4/data_relay"
)

type TestRelay struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

func (p *TestRelay) Start(ctx context.Context, app data_relay.App, config []byte) error {
	return nil
}

func (p *TestRelay) HttpProxy(ctx context.Context, app data_relay.App, t string, header http.Header, data []byte) ([]byte, error) {
	return nil, nil
}
