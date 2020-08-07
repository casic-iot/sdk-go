/*
 * @Descripttion:
 * @version:
 * @Author: zhangqiang
 * @Date: 2020-08-06 11:40:43
 * @LastEditors: zhangqiang
 * @LastEditTime: 2020-08-07 10:58:06
 */
package main

import (
	"math/rand"
	"time"

	"github.com/air-iot/sdk-go/task"
	"github.com/robfig/cron/v3"
)

// TestTask 定义测试任务结构体
type TestTask struct {
	taskIds map[cron.EntryID]int
}

// Start 驱动执行，实现Task的Start函数
func (p *TestTask) Start(a task.App) error {
	p.taskIds = make(map[cron.EntryID]int)
	a.GetLogger().Debugln("start")
	id, _ := a.GetCron().AddFunc("* * * * * *", func() {
		a.GetLogger().Debugln(rand.Int())
	})
	p.taskIds[id] = 0
	go func() {
		time.Sleep(time.Second * 5)
		for id := range p.taskIds {
			a.GetCron().Remove(id)
		}
		id, _ := a.GetCron().AddFunc("* * * * * *", func() {
			a.GetLogger().Debugln(rand.Float64())
		})
		p.taskIds[id] = 0
	}()
	return nil
}

func (p *TestTask) Stop(a task.App) error {
	a.GetLogger().Debugln("stop")
	return nil
}

func main() {
	// 创建任务主程序
	task.NewApp().Start(new(TestTask))
}
