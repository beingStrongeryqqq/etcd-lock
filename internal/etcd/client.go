// internal/etcd/client.go

package etcd

import (
	"context"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.uber.org/zap"

	"etcd-lock/internal/config"
)

// Client ETCD客户端封装
type Client struct {
	cli     *clientv3.Client
	logger  *zap.Logger
	timeout time.Duration
	config  *config.Config // 添加配置字段
}

// Options 客户端配置选项
type Options struct {
	Endpoints   []string
	Username    string
	Password    string
	DialTimeout time.Duration
	Logger      *zap.Logger
	Config      *config.Config // 添加配置选项
}

// NewClient 创建新的 ETCD 客户端
func NewClient(opts Options) (*Client, error) {
	if opts.Logger == nil {
		var err error
		opts.Logger, err = zap.NewProduction()
		if err != nil {
			return nil, err
		}
	}

	if opts.DialTimeout == 0 {
		opts.DialTimeout = 5 * time.Second
	}

	config := clientv3.Config{
		Endpoints:   opts.Endpoints,
		Username:    opts.Username,
		Password:    opts.Password,
		DialTimeout: opts.DialTimeout,
	}

	cli, err := clientv3.New(config)
	if err != nil {
		opts.Logger.Error("failed to create etcd client", zap.Error(err))
		return nil, err
	}

	return &Client{
		cli:     cli,
		logger:  opts.Logger,
		timeout: opts.DialTimeout,
		config:  opts.Config, // 保存配置
	}, nil
}

// Config 返回客户端配置
func (c *Client) Config() *config.Config {
	return c.config
}

// Logger 返回日志实例
func (c *Client) Logger() *zap.Logger {
	return c.logger
}

// Close 关闭客户端连接
func (c *Client) Close() error {
	if c.cli != nil {
		return c.cli.Close()
	}
	return nil
}

// Get 获取键值
func (c *Client) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.cli.Get(ctx, key, opts...)
	if err != nil {
		c.logger.Error("failed to get key",
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// Put 设置键值
func (c *Client) Put(ctx context.Context, key, val string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.cli.Put(ctx, key, val, opts...)
	if err != nil {
		c.logger.Error("failed to put key",
			zap.String("key", key),
			zap.String("value", val),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// Delete 删除键值
func (c *Client) Delete(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.DeleteResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.cli.Delete(ctx, key, opts...)
	if err != nil {
		c.logger.Error("failed to delete key",
			zap.String("key", key),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// Watch 监听键值变化
func (c *Client) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	return c.cli.Watch(ctx, key, opts...)
}

// RawClient 获取原始 ETCD 客户端
func (c *Client) RawClient() *clientv3.Client {
	return c.cli
}

// Grant 创建租约
func (c *Client) Grant(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.cli.Grant(ctx, ttl)
	if err != nil {
		c.logger.Error("failed to grant lease",
			zap.Int64("ttl", ttl),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// Revoke 撤销租约
func (c *Client) Revoke(ctx context.Context, leaseID clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	resp, err := c.cli.Revoke(ctx, leaseID)
	if err != nil {
		c.logger.Error("failed to revoke lease",
			zap.Int64("leaseID", int64(leaseID)),
			zap.Error(err))
		return nil, err
	}
	return resp, nil
}

// KeepAlive 保持租约有效
func (c *Client) KeepAlive(ctx context.Context, leaseID clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	return c.cli.KeepAlive(ctx, leaseID)
}
