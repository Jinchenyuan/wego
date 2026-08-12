package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/Jinchenyuan/wego/transport"
	"github.com/gin-gonic/gin"
)

type Server struct {
	*http.Server
	auth     gin.HandlerFunc
	opts     options
	mu       sync.Mutex
	listener net.Listener
}

func NewHTTPServer(opts ...Options) *Server {
	o := options{}
	for _, opt := range opts {
		opt(&o)
	}

	hs := &Server{
		opts: o,
	}

	r := gin.Default()
	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", o.Port),
		Handler: r,
	}
	hs.Server = srv

	return hs
}

func (s *Server) SetAuthMiddleware(auth gin.HandlerFunc) {
	s.auth = auth
}

func (s *Server) RegisterRoute(method string, path string, handler gin.HandlerFunc) {
	r := s.Handler.(*gin.Engine)
	switch method {
	case http.MethodGet:
		r.GET(path, s.auth, handler)
	case http.MethodPost:
		r.POST(path, s.auth, handler)
	case http.MethodPut:
		r.PUT(path, s.auth, handler)
	case http.MethodDelete:
		r.DELETE(path, s.auth, handler)
	}
}

func (s *Server) GetEngine() *gin.Engine {
	return s.Handler.(*gin.Engine)
}

func (s *Server) GetType() transport.NetType {
	return s.opts.Type
}

func (s *Server) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", s.Addr)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.listener = listener
	s.mu.Unlock()
	go func() {
		_ = s.Serve(listener)
	}()
	go func() { <-ctx.Done(); _ = s.Stop(context.Background()) }()
	return nil
}

func (s *Server) Stop(ctx context.Context) error {
	s.mu.Lock()
	listener := s.listener
	s.listener = nil
	s.mu.Unlock()
	if listener == nil {
		return nil
	}
	err := s.Shutdown(ctx)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
