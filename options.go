package wego

import (
	"github.com/Jinchenyuan/wego/logger"
	"github.com/Jinchenyuan/wego/transport"
	"github.com/Jinchenyuan/wego/transport/micro"
	clientv3 "go.etcd.io/etcd/client/v3"
)

type Options func(o *options)

type RedisConfig struct {
	Addr     string
	Password string
	DB       int
}

type Profile struct {
	Name string
}

type options struct {
	LogLevel logger.Level

	HttpPort int

	EtcdConfig clientv3.Config

	dsn string

	RedisConfig RedisConfig

	Servers []transport.Server

	serviceScheme micro.ServiceScheme

	profile Profile
}

func WithProfile(p Profile) Options {
	return func(o *options) {
		o.profile = p
	}
}

func WithServiceScheme(scheme micro.ServiceScheme) Options {
	return func(o *options) {
		o.serviceScheme = scheme
	}
}

func WithDSN(dsn string) Options {
	return func(o *options) {
		o.dsn = dsn
	}
}

func WithHttpPort(port int) Options {
	return func(o *options) {
		o.HttpPort = port
	}
}

func WithLogLevel(level logger.Level) Options {
	return func(o *options) {
		o.LogLevel = level
	}
}

func WithServers(servers ...transport.Server) Options {
	return func(o *options) {
		o.Servers = servers
	}
}

func WithEtcdConfig(config clientv3.Config) Options {
	return func(o *options) {
		o.EtcdConfig = config
	}
}

func WithRedisConfig(cfg RedisConfig) Options {
	return func(o *options) {
		o.RedisConfig = cfg
	}
}
