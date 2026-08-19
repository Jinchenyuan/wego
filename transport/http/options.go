package http

import (
	"net"
	"time"

	"github.com/Jinchenyuan/wego/telemetry"
	"github.com/Jinchenyuan/wego/transport"
)

type Options func(o *options)

type options struct {
	Host              net.IP
	Port              int
	Type              transport.NetType
	ReadHeaderTimeout time.Duration
	ReadTimeout       time.Duration
	WriteTimeout      time.Duration
	IdleTimeout       time.Duration
	MaxHeaderBytes    int
	MaxBodyBytes      int64
	RequestTimeout    time.Duration
	Telemetry         *telemetry.Runtime
	Metrics           *telemetry.Registry
}

func WithTelemetry(runtime *telemetry.Runtime, metrics *telemetry.Registry) Options {
	return func(o *options) {
		o.Telemetry = runtime
		o.Metrics = metrics
	}
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

func WithTimeouts(readHeader, read, write, idle time.Duration) Options {
	return func(o *options) {
		if readHeader > 0 {
			o.ReadHeaderTimeout = readHeader
		}
		if read > 0 {
			o.ReadTimeout = read
		}
		if write > 0 {
			o.WriteTimeout = write
		}
		if idle > 0 {
			o.IdleTimeout = idle
		}
	}
}

func WithMaxHeaderBytes(n int) Options {
	return func(o *options) {
		if n > 0 {
			o.MaxHeaderBytes = n
		}
	}
}
func WithMaxBodyBytes(n int64) Options {
	return func(o *options) {
		if n > 0 {
			o.MaxBodyBytes = n
		}
	}
}
func WithRequestTimeout(d time.Duration) Options {
	return func(o *options) {
		if d > 0 {
			o.RequestTimeout = d
		}
	}
}
