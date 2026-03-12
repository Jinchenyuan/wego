package http

import (
	"context"
	"fmt"
	"log"
	"net/http"

	"github.com/Jinchenyuan/wego/core/transport"
	"github.com/gin-gonic/gin"
)

type Server struct {
	*http.Server
	auth gin.HandlerFunc
	opts options
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
	default:
		log.Printf("unsupported method %s for path %s\n", method, path)
	}
}

func (s *Server) GetEngine() *gin.Engine {
	return s.Handler.(*gin.Engine)
}

func (s *Server) GetType() transport.NetType {
	return s.opts.Type
}

func (s *Server) Start(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		if err := s.Shutdown(context.Background()); err != nil {
			log.Printf("http server shutdown failed:%s\n", err)
		} else {
			log.Printf("http server shutdown success\n")
		}
	}()

	go func() {
		if err := s.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen %s\n", err)
		}
	}()

	return nil
}
