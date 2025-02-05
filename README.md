# ETCD Watch Lock

基于 ETCD 实现的分布式锁，支持 Watch 机制和自动续约功能。

## 功能特性

- 基本的分布式锁操作（获取、释放）
- Watch 机制监控锁状态变化
- 自动续约
- 支持公平锁和非公平锁
- 可配置的锁超时和重试策略
- 优雅的错误处理
- 完善的日志记录

## 安装要求

- Go 1.20 或更高版本
- ETCD 3.5 或更高版本

## 快速开始

### 安装

```bash
go get github.com/beingStrongeryqqq/etcd-lock
```

### 基本使用

```go
package main

import (
    "context"
    "log"
    "time"
    
    "etcd-lock/internal/config"
    "etcd-lock/internal/etcd"
    "etcd-lock/internal/lock"
)

func main() {
    // 创建配置
    cfg := config.DefaultConfig().
        WithEndpoints([]string{"localhost:2379"})

    // 创建客户端
    client, err := etcd.NewClient(etcd.Options{
        Endpoints: cfg.ETCD.Endpoints,
        Config:   cfg,
    })
    if err != nil {
        log.Fatal(err)
    }
    defer client.Close()

    // 创建租约管理器
    leaseManager := etcd.NewLeaseManager(client, time.Second*5)
    defer leaseManager.Close()

    // 创建锁
    myLock, err := lock.NewLock(
        client,
        leaseManager,
        "/myapp/locks/resource-1",
        "owner-1",
        lock.WithTTL(10*time.Second),
        lock.WithAutoRefresh(true),
    )
    if err != nil {
        log.Fatal(err)
    }

    // 获取锁
    if err := myLock.Lock(context.Background()); err != nil {
        log.Fatal(err)
    }
    defer myLock.Unlock(context.Background())

    // 使用受保护的资源
    // ...
}
```

### Watch 机制使用

```go
type MyHandler struct {
    logger *zap.Logger
}

func (h *MyHandler) OnLockAcquired(event lock.LockEvent) {
    h.logger.Info("lock acquired", zap.String("key", event.Key))
}

func (h *MyHandler) OnLockReleased(event lock.LockEvent) {
    h.logger.Info("lock released", zap.String("key", event.Key))
}

func (h *MyHandler) OnLockExpired(event lock.LockEvent) {
    h.logger.Info("lock expired", zap.String("key", event.Key))
}

func main() {
    // ... 初始化客户端代码省略

    // 创建观察者
    watcher := lock.NewWatcher(client, "/myapp/locks/", 100)
    if err := watcher.Start(); err != nil {
        log.Fatal(err)
    }
    defer watcher.Close()

    // 使用自定义处理器
    handler := &MyHandler{logger: zap.NewExample()}
    watcher.WatchWithHandler(handler)

    // 保持程序运行
    select {}
}
```

## 配置选项

### 锁选项

```go
// 创建锁时的选项
lock.WithTTL(10*time.Second)       // 设置锁的过期时间
lock.WithTimeout(5*time.Second)    // 设置获取锁的超时时间
lock.WithAutoRefresh(true)         // 启用自动续约
lock.WithBlockOnAcquire(true)      // 获取失败时阻塞等待
lock.WithRetryInterval(100*time.Millisecond) // 设置重试间隔
```

### ETCD 配置

```go
config.DefaultConfig().
    WithEndpoints([]string{"localhost:2379"}).
    WithAuth("user", "password").              // 设置认证信息
    WithLockPrefix("/myapp/locks/")            // 设置锁的前缀
```

## 项目结构

```
etcd-lock/
├── cmd/                    # 应用程序入口
├── internal/              # 内部包
│   ├── config/           # 配置管理
│   ├── etcd/            # ETCD 客户端封装
│   └── lock/            # 锁实现
├── examples/             # 使用示例
└── tests/               # 测试
```

## 示例说明

1. basic_lock.go - 基本的锁操作示例
2. auto_release.go - 展示自动续约和优雅退出
3. watch_demo.go - Watch 机制的使用示例

## 注意事项

1. 确保 ETCD 服务可用
2. 合理设置 TTL 和续约间隔
3. 正确处理错误和异常情况
4. 使用 defer 确保锁的释放
5. 避免长时间持有锁

## 贡献指南

1. Fork 项目
2. 创建新的特性分支
3. 提交更改
4. 发起 Pull Request

## 许可证

MIT License

## 联系方式

- 作者：yq
- 邮箱：1747160227@qq.com
- GitHub：github.com/beingStrongeryqqq

## 更新日志

### v1.0.0 (2024-02-05)
- 初始版本发布
- 实现基本的分布式锁功能
- 添加 Watch 机制
- 支持自动续约
