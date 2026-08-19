package micro

import (
	"github.com/Jinchenyuan/wego/telemetry"
	"github.com/Jinchenyuan/wego/transport"
	"go-micro.dev/v5/registry"
)

type Options func(o *options)

type ServiceScheme struct {
	Name    string
	Version string
	Port    int
}

type options struct {
	reg           registry.Registry
	Type          transport.NetType
	serviceScheme ServiceScheme
	telemetry     *telemetry.Runtime
}

func WithTelemetry(runtime *telemetry.Runtime) Options {
	return func(o *options) { o.telemetry = runtime }
}

func WithServiceScheme(scheme ServiceScheme) Options {
	return func(o *options) {
		o.serviceScheme = scheme
	}
}

func WithType(typ transport.NetType) Options {
	return func(o *options) {
		o.Type = typ
	}
}

func WithRegistry(reg registry.Registry) Options {
	return func(o *options) {
		o.reg = reg
	}
}
