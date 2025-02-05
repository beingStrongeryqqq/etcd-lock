// examples/auto_release.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"etcd-lock/internal/config"
	"etcd-lock/internal/etcd"
	"etcd-lock/internal/lock"
)

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	hostname, _ := os.Hostname()

	// 创建上下文，支持优雅退出
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 创建配置和客户端
	cfg := config.DefaultConfig().
		WithEndpoints("localhost:2379")

	client, err := etcd.NewClient(etcd.Options{
		Endpoints: cfg.ETCD.Endpoints,
		Logger:    logger,
		Config:    cfg,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	leaseManager := etcd.NewLeaseManager(client, cfg.ETCD.OpTimeout)
	defer leaseManager.Close()

	// 创建锁，配置短TTL和自动续约
	myLock, err := lock.NewLock(
		client,
		leaseManager,
		"/myapp/locks/auto-release",
		hostname,
		lock.WithTTL(5*time.Second), // 5秒后自动释放
		lock.WithAutoRefresh(true),  // 启用自动续约
		lock.WithOwner(hostname),
		lock.WithNotifications(true, true, true), // 启用所有通知
	)
	if err != nil {
		log.Fatal(err)
	}

	// 尝试获取锁
	fmt.Println("尝试获取锁...")
	if err := myLock.Lock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("成功获取锁")

	// 启动后台goroutine检查锁状态
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !myLock.IsHeld() {
					fmt.Println("锁已丢失")
					return
				}
				// 获取锁信息
				_, ttl, err := myLock.GetLockInformation(ctx)
				if err != nil {
					log.Printf("获取锁信息失败: %v", err)
					continue
				}
				fmt.Printf("锁剩余TTL: %d秒\n", ttl)
			}
		}
	}()

	// 等待信号或超时
	select {
	case sig := <-sigChan:
		fmt.Printf("收到信号: %v，准备释放锁\n", sig)
		if err := myLock.Unlock(ctx); err != nil {
			log.Printf("释放锁失败: %v", err)
		}
	case <-time.After(30 * time.Second):
		fmt.Println("超时，准备释放锁")
		if err := myLock.Unlock(ctx); err != nil {
			log.Printf("释放锁失败: %v", err)
		}
	}

	fmt.Println("程序退出")
}
