// internal/lock/options.go

package lock

import (
	"errors"
	"time"

	"etcd-lock/internal/config"
)

// LockOptions 定义锁的选项
type LockOptions struct {
	// 基础配置
	config *config.Config

	// 锁特定配置
	ttl           int64         // 锁的过期时间（秒）
	timeout       time.Duration // 获取锁的超时时间
	retryInterval time.Duration // 重试间隔
	maxRetries    int           // 最大重试次数

	// 锁的行为配置
	autoRefresh    bool // 是否自动续约
	blockOnAcquire bool // 获取失败时是否阻塞等待
	waitQueue      bool // 是否使用等待队列（公平锁）

	// 锁的元数据
	owner   string // 锁的持有者标识
	purpose string // 锁的用途说明

	// 事件通知
	notifyLost     bool // 是否通知锁丢失事件
	notifyReleased bool // 是否通知锁释放事件
	notifyExpired  bool // 是否通知锁过期事件
	notifyAcquired bool // 是否通知锁获取事件

	// 高级选项
	delayBeforeRetry time.Duration // 重试前的延迟时间
	maxWaitTime      time.Duration // 最大等待时间
}

// Option 定义选项设置函数类型
type Option func(*LockOptions)

// defaultOptions 返回默认选项
func defaultOptions(cfg *config.Config) *LockOptions {
	return &LockOptions{
		config:           cfg,
		ttl:              int64(cfg.Lock.TTL.Seconds()),
		timeout:          cfg.Lock.AcquireTimeout,
		retryInterval:    cfg.Lock.RetryInterval,
		maxRetries:       cfg.Lock.RetryTimes,
		autoRefresh:      cfg.Lock.AutoRefresh,
		blockOnAcquire:   true,
		waitQueue:        false,
		notifyLost:       true,
		notifyReleased:   true,
		notifyExpired:    true,
		delayBeforeRetry: time.Millisecond * 100,
		maxWaitTime:      time.Minute,
	}
}

// WithTTL 设置锁的过期时间
func WithTTL(ttl time.Duration) Option {
	return func(o *LockOptions) {
		o.ttl = int64(ttl.Seconds())
	}
}

// WithTimeout 设置获取锁的超时时间
func WithTimeout(timeout time.Duration) Option {
	return func(o *LockOptions) {
		o.timeout = timeout
	}
}

// WithRetryInterval 设置重试间隔
func WithRetryInterval(interval time.Duration) Option {
	return func(o *LockOptions) {
		o.retryInterval = interval
	}
}

// WithMaxRetries 设置最大重试次数
func WithMaxRetries(retries int) Option {
	return func(o *LockOptions) {
		o.maxRetries = retries
	}
}

// WithAutoRefresh 设置是否自动续约
func WithAutoRefresh(auto bool) Option {
	return func(o *LockOptions) {
		o.autoRefresh = auto
	}
}

// WithBlockOnAcquire 设置是否阻塞等待
func WithBlockOnAcquire(block bool) Option {
	return func(o *LockOptions) {
		o.blockOnAcquire = block
	}
}

// WithWaitQueue 设置是否使用等待队列（公平锁）
func WithWaitQueue(useQueue bool) Option {
	return func(o *LockOptions) {
		o.waitQueue = useQueue
	}
}

// WithOwner 设置锁的持有者标识
func WithOwner(owner string) Option {
	return func(o *LockOptions) {
		o.owner = owner
	}
}

// WithPurpose 设置锁的用途说明
func WithPurpose(purpose string) Option {
	return func(o *LockOptions) {
		o.purpose = purpose
	}
}

// WithNotifications 设置事件通知选项
func WithNotifications(lost, released, expired bool) Option {
	return func(o *LockOptions) {
		o.notifyLost = lost
		o.notifyReleased = released
		o.notifyExpired = expired
	}
}

// WithDelayBeforeRetry 设置重试前的延迟时间
func WithDelayBeforeRetry(delay time.Duration) Option {
	return func(o *LockOptions) {
		o.delayBeforeRetry = delay
	}
}

// WithMaxWaitTime 设置最大等待时间
func WithMaxWaitTime(maxWait time.Duration) Option {
	return func(o *LockOptions) {
		o.maxWaitTime = maxWait
	}
}

// validate 验证选项的合法性
func (o *LockOptions) validate() error {
	if o.ttl <= 0 {
		return errors.New("TTL must be positive")
	}
	if o.timeout <= 0 {
		return errors.New("timeout must be positive")
	}
	if o.retryInterval <= 0 {
		return errors.New("retry interval must be positive")
	}
	if o.maxRetries < 0 {
		return errors.New("max retries cannot be negative")
	}
	if o.delayBeforeRetry < 0 {
		return errors.New("delay before retry cannot be negative")
	}
	if o.maxWaitTime <= 0 {
		return errors.New("max wait time must be positive")
	}
	return nil
}

// newOptions 创建新的选项实例
func newOptions(cfg *config.Config, opts ...Option) (*LockOptions, error) {
	options := defaultOptions(cfg)

	// 应用自定义选项
	for _, opt := range opts {
		opt(options)
	}

	// 验证选项
	if err := options.validate(); err != nil {
		return nil, err
	}

	return options, nil
}
