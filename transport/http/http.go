package http

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Jinchenyuan/wego/logger"
	"github.com/Jinchenyuan/wego/transport"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
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
	r.Use(recovery(), requestID(), telemetryMiddleware(o), requestLimits(o.MaxBodyBytes, o.RequestTimeout), accessLog())
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

func telemetryMiddleware(opts options) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx := c.Request.Context()
		if opts.Telemetry != nil {
			ctx = opts.Telemetry.Propagator().Extract(ctx, propagation.HeaderCarrier(c.Request.Header))
		}
		ctx, span := opts.Telemetry.Tracer("wego/transport/http").Start(ctx, c.Request.Method+" "+c.Request.URL.Path,
			trace.WithSpanKind(trace.SpanKindServer),
			trace.WithAttributes(attribute.String("http.request.method", c.Request.Method)),
		)
		defer span.End()
		c.Request = c.Request.WithContext(ctx)
		start := time.Now()
		c.Next()

		route := c.FullPath()
		if route == "" {
			route = "unmatched"
		}
		status := c.Writer.Status()
		span.SetName(c.Request.Method + " " + route)
		span.SetAttributes(attribute.String("http.route", route), attribute.Int("http.response.status_code", status))
		if status >= http.StatusInternalServerError {
			span.SetStatus(codes.Error, http.StatusText(status))
		}
		labels := map[string]string{"method": c.Request.Method, "route": route, "status": strconv.Itoa(status)}
		opts.Metrics.Inc("wego_http_server_requests_total", labels)
		opts.Metrics.Observe("wego_http_server_request_duration_seconds", labels, time.Since(start).Seconds())
	}
}

func requestID() gin.HandlerFunc {
	return func(c *gin.Context) {
		id := c.GetHeader("X-Request-ID")
		if id == "" {
			id = fmt.Sprintf("req-%d", time.Now().UnixNano())
		}
		c.Header("X-Request-ID", id)
		c.Set("request_id", id)
		c.Request = c.Request.WithContext(logger.ContextWithRequestID(c.Request.Context(), id))
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
				logger.GetLogger("http").ErrorContext(c.Request.Context(), "panic method=", c.Request.Method, " path=", c.Request.URL.Path, " error=", r, " stack=", string(debug.Stack()))
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
		logger.GetLogger("http").InfoContext(c.Request.Context(), "http method=", c.Request.Method, " path=", c.FullPath(), " status=", c.Writer.Status(), " duration_ms=", time.Since(start).Milliseconds(), " request_id=", strings.TrimSpace(c.GetString("request_id")))
	}
}

func (s *Server) SetAuthMiddleware(auth gin.HandlerFunc) {
	s.auth = auth
}

func (s *Server) RegisterRoute(method string, path string, handler gin.HandlerFunc) {
	r := s.Handler.(*gin.Engine)
	handlers := []gin.HandlerFunc{handler}
	if s.auth != nil {
		handlers = append([]gin.HandlerFunc{s.auth}, handlers...)
	}
	switch method {
	case http.MethodGet:
		r.GET(path, handlers...)
	case http.MethodPost:
		r.POST(path, handlers...)
	case http.MethodPut:
		r.PUT(path, handlers...)
	case http.MethodDelete:
		r.DELETE(path, handlers...)
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
