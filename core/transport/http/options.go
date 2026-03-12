package http

import (
	"net"

	"github.com/Jinchenyuan/wego/core/transport"
)

type Options func(o *options)

type options struct {
	Host net.IP
	Port int
	Type transport.NetType
}

func WithType(typ transport.NetType) Options {
	return func(o *options) {
		o.Type = typ
	}
}

func WithHost(host net.IP) Options {
	return func(o *options) {
		o.Host = host
	}
}

func WithPort(port int) Options {
	return func(o *options) {
		o.Port = port
	}
}
