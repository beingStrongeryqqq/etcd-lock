// internal/lock/lock.go

package lock

import (
	"context"
	"errors"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	"etcd-lock/internal/etcd"
)

var (
	ErrLockAcquireFailed = errors.New("failed to acquire lock")
	ErrLockNotHeld       = errors.New("lock not held")
	ErrLockTimeout       = errors.New("lock timeout")
)

// Lock 分布式锁
type Lock struct {
	client       *etcd.Client
	lease        *etcd.Lease
	leaseManager *etcd.LeaseManager
	logger       *zap.Logger

	key    string       // 锁的key
	value  string       // 锁的value，通常包含持有者信息
	opts   *LockOptions // 锁的选项配置
	isHeld bool         // 是否持有锁
}

// NewLock 创建新的分布式锁
func NewLock(client *etcd.Client, leaseManager *etcd.LeaseManager, key string, value string, opts ...Option) (*Lock, error) {
	// 创建并验证选项
	options, err := newOptions(client.Config(), opts...)
	if err != nil {
		return nil, fmt.Errorf("invalid options: %w", err)
	}

	return &Lock{
		client:       client,
		leaseManager: leaseManager,
		logger:       client.Logger(),
		key:          key,
		value:        value,
		opts:         options,
	}, nil
}

// Lock 获取锁
func (l *Lock) Lock(ctx context.Context) error {
	// 设置超时上下文
	ctx, cancel := context.WithTimeout(ctx, l.opts.timeout)
	defer cancel()

	// 创建租约
	lease, err := l.leaseManager.Grant(ctx, l.opts.ttl)
	if err != nil {
		return fmt.Errorf("failed to grant lease: %w", err)
	}

	// 使用事务确保原子性
	txn := l.client.RawClient().Txn(ctx)
	txn = txn.If(
		// 确保key不存在
		clientv3.Compare(clientv3.CreateRevision(l.key), "=", 0),
	).Then(
		// 创建key，设置租约
		clientv3.OpPut(l.key, l.value, clientv3.WithLease(lease.ID)),
	).Else(
		// key已存在，获取其信息
		clientv3.OpGet(l.key),
	)

	resp, err := txn.Commit()
	if err != nil {
		// 撤销租约
		lease.Revoke(ctx)
		return fmt.Errorf("transaction failed: %w", err)
	}

	if !resp.Succeeded {
		// 锁已被占用
		lease.Revoke(ctx)
		if l.opts.blockOnAcquire {
			// 如果配置为阻塞模式，进入重试循环
			return l.retryAcquire(ctx)
		}
		currentHolder := string(resp.Responses[0].GetResponseRange().Kvs[0].Value)
		return fmt.Errorf("%w: held by %s", ErrLockAcquireFailed, currentHolder)
	}

	// 根据配置决定是否自动续约
	if l.opts.autoRefresh {
		if err := lease.KeepAlive(ctx); err != nil {
			lease.Revoke(ctx)
			return fmt.Errorf("failed to start lease keepalive: %w", err)
		}
	}

	l.lease = lease
	l.isHeld = true

	// 根据配置发送通知
	if l.opts.notifyAcquired {
		l.notifyAcquired()
	}

	return nil
}

// retryAcquire 重试获取锁
func (l *Lock) retryAcquire(ctx context.Context) error {
	attempt := 0
	for attempt < l.opts.maxRetries {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(l.opts.retryInterval):
			attempt++
			if err := l.Lock(ctx); err == nil {
				return nil
			}
		}
	}
	return ErrLockTimeout
}

// TryLock 尝试获取锁，立即返回结果
func (l *Lock) TryLock(ctx context.Context) (bool, error) {
	// 保存当前的阻塞设置
	blockOnAcquire := l.opts.blockOnAcquire
	// 临时设置为非阻塞模式
	l.opts.blockOnAcquire = false
	// 恢复原始设置
	defer func() {
		l.opts.blockOnAcquire = blockOnAcquire
	}()

	err := l.Lock(ctx)
	if err == nil {
		return true, nil
	}
	if errors.Is(err, ErrLockAcquireFailed) {
		return false, nil
	}
	return false, err
}

// Unlock 释放锁
func (l *Lock) Unlock(ctx context.Context) error {
	if !l.isHeld {
		return ErrLockNotHeld
	}

	// 撤销租约会自动删除关联的key
	if err := l.lease.Revoke(ctx); err != nil {
		return fmt.Errorf("failed to revoke lease: %w", err)
	}

	l.isHeld = false
	l.lease = nil

	// 根据配置发送通知
	if l.opts.notifyReleased {
		l.notifyReleased()
	}

	return nil
}

// Refresh 刷新锁的TTL
func (l *Lock) Refresh(ctx context.Context) error {
	if !l.isHeld {
		return ErrLockNotHeld
	}

	// 获取租约状态
	resp, err := l.lease.TimeToLive(ctx)
	if err != nil {
		return fmt.Errorf("failed to get lease TTL: %w", err)
	}

	if resp.TTL == 0 {
		l.isHeld = false
		l.lease = nil
		// 根据配置发送通知
		if l.opts.notifyExpired {
			l.notifyExpired()
		}
		return ErrLockNotHeld
	}

	return nil
}

// IsHeld 检查是否持有锁
func (l *Lock) IsHeld() bool {
	return l.isHeld
}

// GetLockInformation 获取锁的信息
func (l *Lock) GetLockInformation(ctx context.Context) (string, int64, error) {
	if !l.isHeld {
		return "", 0, ErrLockNotHeld
	}

	resp, err := l.lease.TimeToLive(ctx)
	if err != nil {
		return "", 0, fmt.Errorf("failed to get lease TTL: %w", err)
	}

	return l.value, resp.TTL, nil
}

// notifyAcquired 发送锁获取通知
func (l *Lock) notifyAcquired() {
	l.logger.Info("lock acquired",
		zap.String("key", l.key),
		zap.String("owner", l.opts.owner),
		zap.String("purpose", l.opts.purpose))
}

// notifyReleased 发送锁释放通知
func (l *Lock) notifyReleased() {
	l.logger.Info("lock released",
		zap.String("key", l.key),
		zap.String("owner", l.opts.owner))
}

// notifyExpired 发送锁过期通知
func (l *Lock) notifyExpired() {
	l.logger.Warn("lock expired",
		zap.String("key", l.key),
		zap.String("owner", l.opts.owner))
}
