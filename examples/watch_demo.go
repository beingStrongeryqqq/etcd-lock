// examples/watch_demo.go

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

// 自定义事件处理器
type MyEventHandler struct {
	logger *zap.Logger
}

func (h *MyEventHandler) OnLockAcquired(event lock.LockEvent) {
	h.logger.Info("锁被获取",
		zap.String("key", event.Key),
		zap.String("owner", event.Value))
}

func (h *MyEventHandler) OnLockReleased(event lock.LockEvent) {
	h.logger.Info("锁被释放",
		zap.String("key", event.Key))
}

func (h *MyEventHandler) OnLockExpired(event lock.LockEvent) {
	h.logger.Warn("锁已过期",
		zap.String("key", event.Key))
}

func main() {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// 创建上下文
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// 监听系统信号
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	// 创建配置和客户端
	cfg := config.DefaultConfig().
		WithEndpoints("localhost:2379").
		WithLockPrefix("/myapp/locks/")

	client, err := etcd.NewClient(etcd.Options{
		Endpoints: cfg.ETCD.Endpoints,
		Logger:    logger,
		Config:    cfg,
	})
	if err != nil {
		log.Fatal(err)
	}
	defer client.Close()

	// 创建观察者
	watcher := lock.NewWatcher(client, "/myapp/locks/", 100)

	// 创建事件处理器
	handler := &MyEventHandler{logger: logger}

	// 启动观察者
	if err := watcher.Start(); err != nil {
		log.Fatal(err)
	}
	defer watcher.Close()

	// 使用事件处理器
	watcher.WatchWithHandler(handler)

	// 启动后台goroutine定期获取当前所有锁的状态
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				holders, err := watcher.GetLockHolders(ctx)
				if err != nil {
					logger.Error("获取锁持有者失败", zap.Error(err))
					continue
				}

				fmt.Println("\n当前锁状态:")
				for key, value := range holders {
					fmt.Printf("锁: %s, 持有者: %s\n", key, value)
				}
			}
		}
	}()

	// 创建一些测试锁
	leaseManager := etcd.NewLeaseManager(client, cfg.ETCD.OpTimeout)
	defer leaseManager.Close()

	// 创建测试用的锁
	for i := 1; i <= 3; i++ {
		lockName := fmt.Sprintf("/myapp/locks/test-%d", i)
		ownerName := fmt.Sprintf("owner-%d", i)

		testLock, err := lock.NewLock(
			client,
			leaseManager,
			lockName,
			ownerName,
			lock.WithTTL(10*time.Second),
			lock.WithAutoRefresh(true),
			lock.WithOwner(ownerName),
		)
		if err != nil {
			log.Printf("创建锁失败: %v", err)
			continue
		}

		// 获取锁
		if err := testLock.Lock(ctx); err != nil {
			log.Printf("获取锁失败: %v", err)
			continue
		}

		// 随机时间后释放锁
		go func(l *lock.Lock) {
			time.Sleep(time.Duration(5+i*5) * time.Second)
			if err := l.Unlock(ctx); err != nil {
				log.Printf("释放锁失败: %v", err)
			}
		}(testLock)
	}

	// 等待信号
	sig := <-sigChan
	fmt.Printf("\n收到信号: %v，准备退出\n", sig)
}
