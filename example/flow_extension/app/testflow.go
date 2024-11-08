package app

import (
	"context"
	"encoding/json"

	"github.com/air-iot/logger"
	flowextionsion "github.com/air-iot/sdk-go/v4/flow_extension"
)

// TestFlow 定义测试驱动结构体
type TestFlow struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

type Input struct {
	Num1 int `json:"num1"`
	Num2 int `json:"num2"`
}

func (p *TestFlow) Schema(ctx context.Context, a flowextionsion.App, locale string) (string, error) {
	logger.Infof("查询schema: %s", locale)
	return schema, nil
}

func (p *TestFlow) Run(ctx context.Context, a flowextionsion.App, input []byte) (map[string]interface{}, error) {
	logger.Infof("执行run,%s", string(input))
	var in Input
	err := json.Unmarshal(input, &in)
	if err != nil {
		return nil, err
	}
	return map[string]interface{}{"num1": in.Num1 + in.Num2}, nil
}
