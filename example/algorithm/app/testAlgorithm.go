package app

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/air-iot/logger"
	"github.com/air-iot/sdk-go/v4/algorithm"
	"math"
	"strconv"
)

var _ algorithm.Service = &TestAlgorithm{}

// TestAlgorithm 定义测试算法结构体
type TestAlgorithm struct {
	Ctx    context.Context
	Cancel context.CancelFunc
}

// Start 算法执行，实现Driver的Start函数
func (p *TestAlgorithm) Start(_ context.Context, _ algorithm.App) error {
	logger.Debugln("start")
	p.Ctx, p.Cancel = context.WithCancel(context.Background())
	return nil
}
func (p *TestAlgorithm) Stop(_ context.Context, _ algorithm.App) error {
	logger.Debugln("stop")
	p.Cancel()
	return nil
}

func (p *TestAlgorithm) Schema(_ context.Context, _ algorithm.App, lang string) (string, error) {
	return Schema, nil
}

// Run 执行指令，实现Driver的Run函数
func (p *TestAlgorithm) Run(_ context.Context, _ algorithm.App, bts []byte) (interface{}, error) {
	logger.Debugln("run")
	if bts != nil {
		logger.Debugln(string(bts))
	}

	var runConfig algorithm.RunConfig
	err := json.Unmarshal(bts, &runConfig)
	if err != nil {
		return nil, err
	}
	switch runConfig.Function {
	case "add":
		{
			if runConfig.Input == nil {
				logger.Errorln("input为空")
				return nil, errors.New("input为空")
			}

			var num1, num2 float64

			{
				num1Raw, ok := runConfig.Input["num1"]
				if !ok {
					logger.Errorln("未找到num1")
					return nil, errors.New("未找到num1")
				}
				switch num1Raw.(type) {
				case string:
					num1, err = strconv.ParseFloat(num1Raw.(string), 10)
					if err != nil {
						logger.Errorln("num1类型错误")
						return nil, errors.New("num1类型错误")
					}
				case float64:
					num1, ok = num1Raw.(float64)
					if !ok {
						logger.Errorln("num1类型错误")
						return nil, errors.New("num1类型错误")
					}
				case map[string]interface{}:

				default:
					logger.Errorln("num1类型错误")
					return nil, errors.New("num1类型错误")
				}
			}
			{
				num2Raw, ok := runConfig.Input["num2"]
				if !ok {
					logger.Errorln("未找到num2")
					return nil, errors.New("未找到num2")
				}
				switch num2Raw.(type) {
				case string:
					num2, err = strconv.ParseFloat(num2Raw.(string), 10)
					if err != nil {
						logger.Errorln("num2类型错误")
						return nil, errors.New("num2类型错误")
					}
				case float64:
					num2, ok = num2Raw.(float64)
					if !ok {
						logger.Errorln("num2类型错误")
						return nil, errors.New("num2类型错误")
					}
				case map[string]interface{}:

				default:
					logger.Errorln("num2类型错误")
					return nil, errors.New("num2类型错误")
				}
			}

			return map[string]float64{"res": num1 + num2}, nil
		}
	case "abs":
		{
			if runConfig.Input == nil {
				logger.Errorln("input为空")
				return nil, errors.New("input为空")
			}

			var num1 float64

			num1Raw, ok := runConfig.Input["num1"]
			if !ok {
				logger.Errorln("未找到num1")
				return nil, errors.New("未找到num1")
			}
			switch num1Raw.(type) {
			case string:
				num1, err = strconv.ParseFloat(num1Raw.(string), 10)
				if err != nil {
					logger.Errorln("num1类型错误")
					return nil, errors.New("num1类型错误")
				}
			case float64:
				num1, ok = num1Raw.(float64)
				if !ok {
					logger.Errorln("num1类型错误")
					return nil, errors.New("num1类型错误")
				}
			case map[string]interface{}:

			default:
				logger.Errorln("num1类型错误")
				return nil, errors.New("num1类型错误")
			}
			return map[string]float64{"res": math.Abs(num1)}, nil

		}
	default:
		logger.Errorln("不支持的方法标识")
		return nil, errors.New("不支持的方法标识")
	}
}
