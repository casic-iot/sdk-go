package etcd

import (
	"context"
	"fmt"
	clientv3 "go.etcd.io/etcd/client/v3"
	"google.golang.org/grpc"
	"os"
	"os/signal"
	"syscall"
	"testing"
	"time"
)

var client *clientv3.Client

func TestMain(m *testing.M) {
	var err error
	client, err = clientv3.New(clientv3.Config{
		Endpoints:        []string{"127.0.0.1:2379"},
		DialTimeout:      time.Second * 20,
		DialOptions:      []grpc.DialOption{grpc.WithBlock()},
		Username:         "root",
		Password:         "dell123",
		AutoSyncInterval: time.Second * 10,
	})
	if err != nil {
		panic(err)
	}
	m.Run()
	if err := client.Close(); err != nil {
		fmt.Println("关闭连接错误", err)
		return
	}
}

func Test_req(t *testing.T) {
	ctx := context.Background()
	k := "/task/test/a1"
	putRes, err := client.Put(ctx, k, "")
	if err != nil {
		t.Errorf("写值错误,%v", err)
		return
	}
	t.Log(putRes)
	delRes, err := client.Delete(ctx, k)
	if err != nil {
		t.Errorf("删除错误,%v", err)
		return
	}
	t.Log(delRes)
}

func Test_watch(t *testing.T) {

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	k := "/task/test"

	getVal, err := client.Get(ctx, k, clientv3.WithPrefix())
	if err != nil {
		fmt.Println("Get err", err)
		return
	}
	fmt.Println("get val", getVal)
	wc := client.Watch(ctx, k, clientv3.WithPrefix())
	go func() {
		for {
			select {
			case <-ctx.Done():
				fmt.Println("ctx done")
				return
			case resp := <-wc:
				fmt.Println("watch,", resp)
				for _, event := range resp.Events {
					fmt.Println("event", event)
				}
			}
		}

	}()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sc
	t.Log("信号", sig)
}

func Test_req1(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	lease, err := client.Lease.Grant(ctx, 6)
	if err != nil {
		t.Errorf("lease错误,%v", err)
		return
	}
	defer func() {
		// 释放租约
		_, err := client.Revoke(ctx, lease.ID)
		if err != nil {
			fmt.Println("lease释放错误,", err)
			return
		}
	}()
	k := "/task/test/a1"
	putRes, err := client.Put(ctx, k, "12", clientv3.WithLease(lease.ID))
	if err != nil {
		t.Errorf("写值错误,%v", err)
		return
	}
	t.Log(putRes)
	go func() {
		alive, err := client.Lease.KeepAlive(ctx, lease.ID)
		if err != nil {
			fmt.Println("lease续租错误,", err)
			return
		}

		for {
			select {
			case <-ctx.Done():
				fmt.Println("ctx done")
				return
			case resp := <-alive:
				fmt.Println("lease续租,", resp, resp.TTL, time.Now().Local())
			}
		}

	}()
	sc := make(chan os.Signal, 1)
	signal.Notify(sc, syscall.SIGHUP, syscall.SIGINT, syscall.SIGTERM, syscall.SIGQUIT)
	sig := <-sc
	t.Log("信号", sig)
	//go func() {
	//	for {
	//		select {
	//		case <-ctx.Done():
	//			fmt.Println("ctx done")
	//			return
	//		default:
	//			fmt.Println("lease续租", time.Now().Local())
	//			// 每2秒续租一次,保持租约有效
	//			resp, err := client.KeepAliveOnce(context.Background(), lease.ID)
	//			if err != nil {
	//				fmt.Println("lease续租错误,", err, time.Now().Local())
	//			} else {
	//				fmt.Println("lease续租,", resp, resp.TTL, time.Now().Local())
	//			}
	//			time.Sleep(2 * time.Second)
	//		}
	//	}
	//}()
	//delRes, err := client.Delete(ctx, k)
	//if err != nil {
	//	t.Errorf("删除错误,%v", err)
	//	return
	//}
	//t.Log(delRes)

}
