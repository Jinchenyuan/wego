package http

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

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
	o := options{ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 1 << 20, MaxBodyBytes: 10 << 20, RequestTimeout: 30 * time.Second}
	for _, opt := range opts {
		opt(&o)
	}

	hs := &Server{
		opts: o,
	}

	r := gin.New()
	r.Use(recovery(), requestID(), requestLimits(o.MaxBodyBytes, o.RequestTimeout), accessLog())
	srv := &http.Server{
		Addr:              fmt.Sprintf(":%d", o.Port),
		Handler:           r,
		ReadHeaderTimeout: o.ReadHeaderTimeout,
		ReadTimeout:       o.ReadTimeout,
		WriteTimeout:      o.WriteTimeout,
		IdleTimeout:       o.IdleTimeout,
		MaxHeaderBytes:    o.MaxHeaderBytes,
	}
	hs.Server = srv

	return hs
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Next()
	}
}
func requestLimits(maxBody int64, timeout time.Duration) gin.HandlerFunc {
	return func(c *gin.Context) {
		if maxBody > 0 {
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, maxBody)
		}
		if timeout > 0 {
			ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
			defer cancel()
			c.Request = c.Request.WithContext(ctx)
		}
		c.Next()
	}
}
func recovery() gin.HandlerFunc {
	return func(c *gin.Context) {
		defer func() {
			if r := recover(); r != nil {
				log.Printf("panic method=%s path=%s error=%v stack=%s", c.Request.Method, c.Request.URL.Path, r, debug.Stack())
				c.AbortWithStatus(http.StatusInternalServerError)
			}
		}()
		c.Next()
	}
}
func accessLog() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		log.Printf("http method=%s path=%s status=%d duration_ms=%d request_id=%s", c.Request.Method, c.FullPath(), c.Writer.Status(), time.Since(start).Milliseconds(), strings.TrimSpace(c.GetString("request_id")))
	}
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
