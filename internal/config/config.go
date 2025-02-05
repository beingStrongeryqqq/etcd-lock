// internal/config/config.go

package config

import (
	"crypto/tls"
	"errors"
	"time"
)

// Config 配置根结构
type Config struct {
	ETCD  ETCDConfig  `json:"etcd"`
	Lock  LockConfig  `json:"lock"`
	Watch WatchConfig `json:"watch"`
}

// ETCDConfig ETCD 连接配置
type ETCDConfig struct {
	// ETCD 节点地址列表
	Endpoints []string `json:"endpoints"`

	// 认证信息
	Username string `json:"username"`
	Password string `json:"password"`

	// TLS 配置
	TLS *tls.Config `json:"tls,omitempty"`

	// 连接超时时间
	DialTimeout time.Duration `json:"dialTimeout"`

	// 操作超时时间
	OpTimeout time.Duration `json:"opTimeout"`

	// 自动重连
	AutoReconnect bool `json:"autoReconnect"`

	// 重试配置
	MaxRetries    int           `json:"maxRetries"`
	RetryInterval time.Duration `json:"retryInterval"`
}

// LockConfig 分布式锁配置
type LockConfig struct {
	// 锁前缀，所有的锁key都将以此为前缀
	KeyPrefix string `json:"keyPrefix"`

	// 默认的锁过期时间
	TTL time.Duration `json:"ttl"`

	// 获取锁的超时时间
	AcquireTimeout time.Duration `json:"acquireTimeout"`

	// 锁续约配置
	AutoRefresh     bool          `json:"autoRefresh"`
	RefreshInterval time.Duration `json:"refreshInterval"`
	RefreshDuration time.Duration `json:"refreshDuration"`

	// 重试配置
	RetryTimes    int           `json:"retryTimes"`
	RetryInterval time.Duration `json:"retryInterval"`

	// 锁值的格式
	ValueFormat string `json:"valueFormat"`
}

// WatchConfig Watch 机制配置
type WatchConfig struct {
	// 事件通道缓冲区大小
	BufferSize int `json:"bufferSize"`

	// 事件分发超时时间
	EventTimeout time.Duration `json:"eventTimeout"`

	// 是否压缩重复事件
	CompactDuplicates bool `json:"compactDuplicates"`

	// 重新连接配置
	ReconnectInterval time.Duration `json:"reconnectInterval"`
	MaxReconnectTries int           `json:"maxReconnectTries"`
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		ETCD: ETCDConfig{
			Endpoints:     []string{"localhost:2379"},
			DialTimeout:   5 * time.Second,
			OpTimeout:     3 * time.Second,
			AutoReconnect: true,
			MaxRetries:    3,
			RetryInterval: time.Second,
		},
		Lock: LockConfig{
			KeyPrefix:       "/locks/",
			TTL:             10 * time.Second,
			AcquireTimeout:  5 * time.Second,
			AutoRefresh:     true,
			RefreshInterval: time.Second,
			RefreshDuration: 8 * time.Second,
			RetryTimes:      3,
			RetryInterval:   100 * time.Millisecond,
			ValueFormat:     "%s-%d", // hostname-pid 格式
		},
		Watch: WatchConfig{
			BufferSize:        100,
			EventTimeout:      time.Second,
			CompactDuplicates: true,
			ReconnectInterval: 5 * time.Second,
			MaxReconnectTries: 3,
		},
	}
}

// Validate 验证配置的合法性
func (c *Config) Validate() error {
	if err := c.validateETCDConfig(); err != nil {
		return err
	}
	if err := c.validateLockConfig(); err != nil {
		return err
	}
	if err := c.validateWatchConfig(); err != nil {
		return err
	}
	return nil
}

// validateETCDConfig 验证 ETCD 配置
func (c *Config) validateETCDConfig() error {
	if len(c.ETCD.Endpoints) == 0 {
		return errors.New("etcd endpoints cannot be empty")
	}
	if c.ETCD.DialTimeout <= 0 {
		return errors.New("etcd dial timeout must be positive")
	}
	if c.ETCD.OpTimeout <= 0 {
		return errors.New("etcd operation timeout must be positive")
	}
	return nil
}

// validateLockConfig 验证锁配置
func (c *Config) validateLockConfig() error {
	if c.Lock.KeyPrefix == "" {
		return errors.New("lock key prefix cannot be empty")
	}
	if c.Lock.TTL <= 0 {
		return errors.New("lock TTL must be positive")
	}
	if c.Lock.AcquireTimeout <= 0 {
		return errors.New("lock acquire timeout must be positive")
	}
	if c.Lock.AutoRefresh {
		if c.Lock.RefreshInterval <= 0 {
			return errors.New("refresh interval must be positive when auto refresh is enabled")
		}
		if c.Lock.RefreshDuration <= 0 {
			return errors.New("refresh duration must be positive when auto refresh is enabled")
		}
	}
	return nil
}

// validateWatchConfig 验证 Watch 配置
func (c *Config) validateWatchConfig() error {
	if c.Watch.BufferSize <= 0 {
		return errors.New("watch buffer size must be positive")
	}
	if c.Watch.EventTimeout <= 0 {
		return errors.New("watch event timeout must be positive")
	}
	if c.Watch.ReconnectInterval <= 0 {
		return errors.New("watch reconnect interval must be positive")
	}
	return nil
}

// WithEndpoints 设置 ETCD 节点地址
func (c *Config) WithEndpoints(endpoints ...string) *Config {
	c.ETCD.Endpoints = endpoints
	return c
}

// WithAuth 设置认证信息
func (c *Config) WithAuth(username, password string) *Config {
	c.ETCD.Username = username
	c.ETCD.Password = password
	return c
}

// WithTLS 设置 TLS 配置
func (c *Config) WithTLS(tlsConfig *tls.Config) *Config {
	c.ETCD.TLS = tlsConfig
	return c
}

// WithLockPrefix 设置锁前缀
func (c *Config) WithLockPrefix(prefix string) *Config {
	c.Lock.KeyPrefix = prefix
	return c
}

// WithLockTTL 设置锁 TTL
func (c *Config) WithLockTTL(ttl time.Duration) *Config {
	c.Lock.TTL = ttl
	return c
}
