// examples/basic_lock.go

package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"time"

	"go.uber.org/zap"

	"etcd-lock/internal/config"
	"etcd-lock/internal/etcd"
	"etcd-lock/internal/lock"
)

func main() {
	// 创建logger
	logger, err := zap.NewDevelopment()
	if err != nil {
		log.Fatal(err)
	}
	defer logger.Sync()

	// 获取主机名作为锁的持有者标识
	hostname, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}

	// 创建基础配置
	cfg := config.DefaultConfig().
		WithEndpoints("localhost:2379").
		WithLockPrefix("/myapp/locks/")

	// 创建ETCD客户端
	client, err := etcd.NewClient(etcd.Options{
		Endpoints:   cfg.ETCD.Endpoints,
		Username:    cfg.ETCD.Username,
		Password:    cfg.ETCD.Password,
		DialTimeout: cfg.ETCD.DialTimeout,
		Logger:      logger,
		Config:      cfg,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 创建租约管理器
	leaseManager := etcd.NewLeaseManager(client, cfg.ETCD.OpTimeout)
	defer leaseManager.Close()

	// 创建分布式锁
	myLock, err := lock.NewLock(
		client,
		leaseManager,
		"/myapp/locks/resource-1",
		hostname,
		lock.WithTTL(10*time.Second),
		lock.WithTimeout(5*time.Second),
		lock.WithAutoRefresh(true),
		lock.WithOwner(hostname),
		lock.WithPurpose("demo"),
	)
	if err != nil {
		log.Fatal(err)
	}

	// 获取锁
	ctx := context.Background()
	fmt.Println("尝试获取锁...")

	if err := myLock.Lock(ctx); err != nil {
		log.Fatal(err)
	}

	fmt.Println("成功获取锁")

	// 模拟执行一些需要同步的操作
	fmt.Println("执行需要同步的操作...")
	time.Sleep(5 * time.Second)

	// 检查锁状态
	if myLock.IsHeld() {
		fmt.Println("锁仍然持有")

		// 获取锁信息
		owner, ttl, err := myLock.GetLockInformation(ctx)
		if err != nil {
			log.Printf("获取锁信息失败: %v", err)
		} else {
			fmt.Printf("锁持有者: %s, TTL: %d秒\n", owner, ttl)
		}
	}

	// 释放锁
	fmt.Println("释放锁...")
	if err := myLock.Unlock(ctx); err != nil {
		log.Fatal(err)
	}
	fmt.Println("锁已释放")
}
