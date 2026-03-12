package transport

import (
	"context"
)

type Server interface {
	Start(context.Context) error
	GetType() NetType
}
