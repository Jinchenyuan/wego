package etcd

import (
	"context"
	"fmt"
	"time"

	clientv3 "go.etcd.io/etcd/client/v3"
	"go.etcd.io/etcd/client/v3/concurrency"
	"google.golang.org/grpc"
)

type Ctl struct {
	cfg        ClientConfig
	endpoints  []string
	authConfig AuthConfig
	client     *clientv3.Client
}

func NewCtl(cfg ClientConfig, opts ...ClientOption) (*Ctl, error) {
	ctl := &Ctl{
		cfg:       cfg,
		endpoints: []string{},
	}

	for _, opt := range opts {
		opt(ctl)
	}

	if ctl.authConfig.Empty() {
		return nil, fmt.Errorf("etcd auth config is empty")
	}

	client, err := clientv3.New(clientv3.Config{
		Endpoints:   ctl.endpoints,
		DialTimeout: 5 * time.Second,
		DialOptions: []grpc.DialOption{grpc.WithBlock()},
		Username:    ctl.authConfig.Username,
		Password:    ctl.authConfig.Password,
	})
	if err != nil {
		return nil, err
	}
	ctl.client = client

	return ctl, nil
}

func WithAuth(userName, password string) ClientOption {
	return func(c any) {
		ctl := c.(*Ctl)
		ctl.authConfig.Username = userName
		ctl.authConfig.Password = password
	}
}

func WithEndpoints(endpoints []string) ClientOption {
	return func(c any) {
		ctl := c.(*Ctl)
		ctl.endpoints = endpoints
	}
}

func (c *Ctl) Close() {
	if c.client != nil {
		c.client.Close()
	}
}

func (c *Ctl) Put(ctx context.Context, key, value string, opts ...clientv3.OpOption) (*clientv3.PutResponse, error) {
	return c.client.Put(ctx, key, value, opts...)
}

func (c *Ctl) Get(ctx context.Context, key string, opts ...clientv3.OpOption) (*clientv3.GetResponse, error) {
	return c.client.Get(ctx, key, opts...)
}

func (c *Ctl) Leases(ctx context.Context) (*clientv3.LeaseLeasesResponse, error) {
	return c.client.Leases(ctx)
}

func (c *Ctl) GrantLease(ctx context.Context, ttl int64) (*clientv3.LeaseGrantResponse, error) {
	return c.client.Grant(ctx, ttl)
}

func (c *Ctl) TimeToLive(ctx context.Context, leaseID clientv3.LeaseID, opts ...clientv3.LeaseOption) (*clientv3.LeaseTimeToLiveResponse, error) {
	return c.client.TimeToLive(ctx, leaseID, opts...)
}

func (c *Ctl) KeepAlive(ctx context.Context, leaseID clientv3.LeaseID) (<-chan *clientv3.LeaseKeepAliveResponse, error) {
	return c.client.KeepAlive(ctx, leaseID)
}

func (c *Ctl) Revoke(ctx context.Context, leaseID clientv3.LeaseID) (*clientv3.LeaseRevokeResponse, error) {
	return c.client.Revoke(ctx, leaseID)
}

func (c *Ctl) NewLockedMutex(ctx context.Context, key string) (*concurrency.Mutex, error) {
	s, err := concurrency.NewSession(c.client, concurrency.WithTTL(10))
	if err != nil {
		return nil, err
	}
	lock := concurrency.NewMutex(s, key)
	if err := lock.TryLock(ctx); err != nil {
		return nil, err
	}
	return lock, nil
}

func (c *Ctl) Watch(ctx context.Context, key string, opts ...clientv3.OpOption) clientv3.WatchChan {
	return c.client.Watch(ctx, key, opts...)
}
