package micro

import (
	"github.com/Jinchenyuan/wego/core/transport"
	"go-micro.dev/v5/registry"
)

type Options func(o *options)

type ServiceScheme struct {
	Name    transport.ServiceType
	Version string
	Port    int
}

type options struct {
	reg           registry.Registry
	Type          transport.NetType
	serviceScheme ServiceScheme
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
