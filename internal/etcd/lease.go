// internal/etcd/lease.go

package etcd

import (
	"context"
	"sync"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	"etcd-lock/internal/config"
)

// LeaseManager 租约管理器
type LeaseManager struct {
	client  *Client
	logger  *zap.Logger
	config  *config.Config
	leases  sync.Map      // 存储所有活跃的租约
	timeout time.Duration // 操作超时时间
}

// Lease 租约包装
type Lease struct {
	ID      clientv3.LeaseID
	ttl     int64
	stopCh  chan struct{} // 用于停止自动续约
	done    chan struct{} // 用于等待续约goroutine结束
	manager *LeaseManager
	config  *config.Config
}

// NewLeaseManager 创建新的租约管理器
func NewLeaseManager(client *Client, timeout time.Duration) *LeaseManager {
	if timeout == 0 {
		timeout = client.Config().ETCD.OpTimeout
	}

	return &LeaseManager{
		client:  client,
		logger:  client.Logger(),
		config:  client.Config(),
		timeout: timeout,
	}
}

// Grant 创建新的租约
func (lm *LeaseManager) Grant(ctx context.Context, ttl int64) (*Lease, error) {
	// 设置默认的TTL
	if ttl == 0 {
		ttl = int64(lm.config.Lock.TTL.Seconds())
	}

	// 创建租约
	lease, err := lm.client.Grant(ctx, ttl)
	if err != nil {
		lm.logger.Error("failed to grant lease",
			zap.Int64("ttl", ttl),
			zap.Error(err))
		return nil, err
	}

	// 创建租约包装器
	l := &Lease{
		ID:      lease.ID,
		ttl:     ttl,
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		manager: lm,
		config:  lm.config,
	}

	// 存储租约
	lm.leases.Store(lease.ID, l)

	return l, nil
}

// KeepAlive 开始自动续约
func (l *Lease) KeepAlive(ctx context.Context) error {
	// 根据配置设置keepalive间隔
	keepAliveInterval := l.config.Lock.RefreshInterval
	if keepAliveInterval == 0 {
		keepAliveInterval = time.Duration(l.ttl/3) * time.Second
	}

	// 获取租约的keepalive通道
	keepAliveCh, err := l.manager.client.KeepAlive(ctx, l.ID)
	if err != nil {
		l.manager.logger.Error("failed to keep alive lease",
			zap.Int64("leaseID", int64(l.ID)),
			zap.Error(err))
		return err
	}

	// 启动goroutine处理续约
	go func() {
		defer close(l.done)
		defer l.manager.leases.Delete(l.ID)

		ticker := time.NewTicker(keepAliveInterval)
		defer ticker.Stop()

		for {
			select {
			case resp, ok := <-keepAliveCh:
				if !ok {
					// keepalive通道关闭
					l.manager.logger.Warn("lease keepalive channel closed",
						zap.Int64("leaseID", int64(l.ID)))
					return
				}
				if resp == nil {
					// 续约失败
					l.manager.logger.Error("lease keepalive failed",
						zap.Int64("leaseID", int64(l.ID)))
					return
				}
				l.manager.logger.Debug("lease kept alive",
					zap.Int64("leaseID", int64(l.ID)),
					zap.Int64("ttl", resp.TTL))

			case <-ticker.C:
				// 根据配置检查是否需要续约
				if l.config.Lock.AutoRefresh {
					ttl, err := l.TimeToLive(ctx)
					if err != nil {
						l.manager.logger.Error("failed to get lease TTL",
							zap.Int64("leaseID", int64(l.ID)),
							zap.Error(err))
						continue
					}
					if ttl.TTL < int64(l.config.Lock.RefreshDuration.Seconds()) {
						// 需要续约
						l.manager.logger.Debug("refreshing lease",
							zap.Int64("leaseID", int64(l.ID)),
							zap.Int64("currentTTL", ttl.TTL))
					}
				}

			case <-l.stopCh:
				// 手动停止续约
				l.manager.logger.Info("lease keepalive stopped",
					zap.Int64("leaseID", int64(l.ID)))
				return

			case <-ctx.Done():
				// 上下文取消
				l.manager.logger.Info("lease keepalive context done",
					zap.Int64("leaseID", int64(l.ID)))
				return
			}
		}
	}()

	return nil
}

// Revoke 撤销租约
func (l *Lease) Revoke(ctx context.Context) error {
	// 停止自动续约
	close(l.stopCh)
	// 等待续约goroutine结束
	<-l.done

	// 使用配置的超时时间创建新的上下文
	ctx, cancel := context.WithTimeout(ctx, l.config.ETCD.OpTimeout)
	defer cancel()

	// 撤销租约
	_, err := l.manager.client.Revoke(ctx, l.ID)
	if err != nil {
		l.manager.logger.Error("failed to revoke lease",
			zap.Int64("leaseID", int64(l.ID)),
			zap.Error(err))
		return err
	}

	// 从管理器中移除租约
	l.manager.leases.Delete(l.ID)
	return nil
}

// TimeToLive 获取租约剩余时间
func (l *Lease) TimeToLive(ctx context.Context) (*clientv3.LeaseTimeToLiveResponse, error) {
	// 使用配置的超时时间
	ctx, cancel := context.WithTimeout(ctx, l.config.ETCD.OpTimeout)
	defer cancel()

	resp, err := l.manager.client.RawClient().TimeToLive(ctx, l.ID)
	if err != nil {
		l.manager.logger.Error("failed to get lease time to live",
			zap.Int64("leaseID", int64(l.ID)),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// GetLease 获取指定ID的租约
func (lm *LeaseManager) GetLease(leaseID clientv3.LeaseID) (*Lease, bool) {
	value, ok := lm.leases.Load(leaseID)
	if !ok {
		return nil, false
	}
	return value.(*Lease), true
}

// Close 关闭租约管理器
func (lm *LeaseManager) Close() {
	// 撤销所有活跃的租约
	lm.leases.Range(func(key, value interface{}) bool {
		lease := value.(*Lease)
		ctx, cancel := context.WithTimeout(context.Background(), lm.timeout)
		defer cancel()

		if err := lease.Revoke(ctx); err != nil {
			lm.logger.Error("failed to revoke lease during cleanup",
				zap.Int64("leaseID", int64(lease.ID)),
				zap.Error(err))
		}
		return true
	})
}
