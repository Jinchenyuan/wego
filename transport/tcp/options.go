package tcp

import (
	"net"
	"time"

	"github.com/Jinchenyuan/wego/transport"
)

type Options func(o *options)

type options struct {
	Host         net.IP
	Port         int
	Type         transport.NetType
	ReadTimeout  time.Duration
	WriteTimeout time.Duration
	IdleTimeout  time.Duration
	Handler      Handler
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

func WithReadTimeout(timeout time.Duration) Options {
	return func(o *options) {
		o.ReadTimeout = timeout
	}
}

func WithWriteTimeout(timeout time.Duration) Options {
	return func(o *options) {
		o.WriteTimeout = timeout
	}
}

func WithIdleTimeout(timeout time.Duration) Options {
	return func(o *options) {
		o.IdleTimeout = timeout
	}
}

func WithHandler(handler Handler) Options {
	return func(o *options) {
		o.Handler = handler
	}
}
