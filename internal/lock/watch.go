// internal/lock/watch.go

package lock

import (
	"context"
	"fmt"
	"sync"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	"etcd-lock/internal/etcd"
)

// EventType 事件类型
type EventType int

const (
	EventAcquired EventType = iota // 锁被获取
	EventReleased                  // 锁被释放
	EventExpired                   // 锁过期
)

// LockEvent 锁事件
type LockEvent struct {
	Type     EventType `json:"type"`
	Key      string    `json:"key"`
	Value    string    `json:"value"`    // 锁持有者信息
	Revision int64     `json:"revision"` // ETCD修订版本
}

// Watcher 锁观察者
type Watcher struct {
	client *etcd.Client
	logger *zap.Logger
	prefix string // 要监听的key前缀
	ctx    context.Context
	cancel context.CancelFunc
	events chan LockEvent
	wg     sync.WaitGroup
}

// NewWatcher 创建新的观察者
func NewWatcher(client *etcd.Client, prefix string, bufferSize int) *Watcher {
	if bufferSize <= 0 {
		bufferSize = 100 // 默认缓冲区大小
	}

	ctx, cancel := context.WithCancel(context.Background())

	return &Watcher{
		client: client,
		logger: client.Logger(),
		prefix: prefix,
		ctx:    ctx,
		cancel: cancel,
		events: make(chan LockEvent, bufferSize),
	}
}

// Start 开始监听
func (w *Watcher) Start() error {
	// 获取当前的锁状态
	resp, err := w.client.Get(w.ctx, w.prefix, clientv3.WithPrefix())
	if err != nil {
		return fmt.Errorf("failed to get initial state: %w", err)
	}

	// 记录当前所有锁的状态
	for _, kv := range resp.Kvs {
		w.events <- LockEvent{
			Type:     EventAcquired,
			Key:      string(kv.Key),
			Value:    string(kv.Value),
			Revision: kv.ModRevision,
		}
	}

	// 开始监听变更
	watchChan := w.client.Watch(w.ctx, w.prefix, clientv3.WithPrefix(), clientv3.WithRev(resp.Header.Revision+1))

	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		w.watchLoop(watchChan)
	}()

	return nil
}

// watchLoop 处理监听循环
func (w *Watcher) watchLoop(watchChan clientv3.WatchChan) {
	for {
		select {
		case <-w.ctx.Done():
			return

		case resp, ok := <-watchChan:
			if !ok {
				w.logger.Error("watch channel closed")
				return
			}

			if err := resp.Err(); err != nil {
				w.logger.Error("watch error", zap.Error(err))
				continue
			}

			// 处理所有事件
			for _, event := range resp.Events {
				w.handleWatchEvent(event)
			}
		}
	}
}

// handleWatchEvent 处理单个监听事件
func (w *Watcher) handleWatchEvent(event *clientv3.Event) {
	var lockEvent LockEvent

	switch event.Type {
	case clientv3.EventTypePut:
		// key被创建或更新
		lockEvent = LockEvent{
			Type:     EventAcquired,
			Key:      string(event.Kv.Key),
			Value:    string(event.Kv.Value),
			Revision: event.Kv.ModRevision,
		}

	case clientv3.EventTypeDelete:
		// key被删除（锁释放或过期）
		lockEvent = LockEvent{
			Type:     EventReleased,
			Key:      string(event.Kv.Key),
			Revision: event.Kv.ModRevision,
		}

		// 判断是否是过期导致的删除
		if event.PrevKv != nil {
			lease := event.PrevKv.Lease
			if lease > 0 {
				lockEvent.Type = EventExpired
			}
		}
	}

	// 发送事件
	select {
	case w.events <- lockEvent:
		w.logger.Debug("lock event sent",
			zap.String("type", lockEvent.Type.String()),
			zap.String("key", lockEvent.Key))
	default:
		w.logger.Warn("event channel full, dropping event",
			zap.String("type", lockEvent.Type.String()),
			zap.String("key", lockEvent.Key))
	}
}

// Events 返回事件通道
func (w *Watcher) Events() <-chan LockEvent {
	return w.events
}

// Close 关闭观察者
func (w *Watcher) Close() error {
	w.cancel()
	w.wg.Wait()
	close(w.events)
	return nil
}

// String 将事件类型转换为字符串
func (t EventType) String() string {
	switch t {
	case EventAcquired:
		return "acquired"
	case EventReleased:
		return "released"
	case EventExpired:
		return "expired"
	default:
		return "unknown"
	}
}

// GetLockHolders 获取当前所有锁持有者
func (w *Watcher) GetLockHolders(ctx context.Context) (map[string]string, error) {
	resp, err := w.client.Get(ctx, w.prefix, clientv3.WithPrefix())
	if err != nil {
		return nil, fmt.Errorf("failed to get lock holders: %w", err)
	}

	holders := make(map[string]string)
	for _, kv := range resp.Kvs {
		holders[string(kv.Key)] = string(kv.Value)
	}

	return holders, nil
}

// LockEventHandler 锁事件处理器接口
type LockEventHandler interface {
	OnLockAcquired(event LockEvent)
	OnLockReleased(event LockEvent)
	OnLockExpired(event LockEvent)
}

// WatchWithHandler 使用处理器监听锁事件
func (w *Watcher) WatchWithHandler(handler LockEventHandler) {
	w.wg.Add(1)
	go func() {
		defer w.wg.Done()
		for event := range w.events {
			switch event.Type {
			case EventAcquired:
				handler.OnLockAcquired(event)
			case EventReleased:
				handler.OnLockReleased(event)
			case EventExpired:
				handler.OnLockExpired(event)
			}
		}
	}()
}
