package micro

import (
	"context"
	"fmt"
	"log"

	"github.com/Jinchenyuan/wego/transport"
	goMicro "go-micro.dev/v5"
	"go-micro.dev/v5/registry"
)

type RegisterHandler func(goMicro.Service) error
type NewServiceClients func(reg registry.Registry) map[string]any

type Service struct {
	opts    options
	clients map[string]any
	service goMicro.Service
}

func NewMicroServer(opts ...Options) *Service {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	ms := &Service{
		opts: o,
	}

	return ms
}

func (s *Service) GetServiceClient(service string) any {
	if s.clients == nil {
		return nil
	}
	if _, ok := s.clients[service]; !ok {
		return nil
	}
	return s.clients[service]
}

func (s *Service) NewServiceClients(nsc NewServiceClients) {
	s.clients = nsc(s.opts.reg)
}

func (s *Service) RegisterHandler(handler RegisterHandler) {
	s.service = goMicro.NewService(
		goMicro.Name(string(s.opts.serviceScheme.Name)),
		goMicro.Address(fmt.Sprintf(":%d", s.opts.serviceScheme.Port)),
		goMicro.Registry(s.opts.reg),
	)
	s.service.Init()
	if err := handler(s.service); err != nil {
		log.Fatalf("RegisterHandler err: %v", err)
	}
}

func (s *Service) GetType() transport.NetType {
	return s.opts.Type
}

func (s *Service) Start(ctx context.Context) error {
	if s.service == nil {
		return nil
	}
	if err := s.service.Server().Start(); err != nil {
		return err
	}

	go func() {
		<-ctx.Done()
		if err := s.service.Server().Stop(); err != nil {
			fmt.Printf("micro service stop err: %v\n", err)
		}
		s.service = nil
	}()
	return nil
}
