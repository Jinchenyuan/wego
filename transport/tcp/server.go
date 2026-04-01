package tcp

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/Jinchenyuan/wego/transport"
)

var ErrHandlerRequired = errors.New("tcp handler is required")
var ErrServerStarted = errors.New("tcp server already started")

type Handler func(context.Context, net.Conn)

type Server struct {
	opts     options
	listener net.Listener
	handler  Handler

	mu      sync.RWMutex
	conns   map[net.Conn]struct{}
	closeCh chan struct{}
	once    sync.Once
}

func NewTCPServer(opts ...Options) *Server {
	o := options{
		Type: transport.TCP,
	}
	for _, opt := range opts {
		opt(&o)
	}

	return &Server{
		opts:    o,
		handler: o.Handler,
		conns:   make(map[net.Conn]struct{}),
		closeCh: make(chan struct{}),
	}
}

func (s *Server) SetHandler(handler Handler) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.handler = handler
	s.opts.Handler = handler
}

func (s *Server) Addr() net.Addr {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.listener == nil {
		return nil
	}
	return s.listener.Addr()
}

func (s *Server) GetType() transport.NetType {
	return s.opts.Type
}

func (s *Server) Start(ctx context.Context) error {
	handler := s.getHandler()
	if handler == nil {
		return ErrHandlerRequired
	}

	listener, err := net.Listen("tcp", s.listenAddr())
	if err != nil {
		return err
	}

	s.mu.Lock()
	if s.listener != nil {
		s.mu.Unlock()
		_ = listener.Close()
		return ErrServerStarted
	}
	if s.opts.Type == 0 {
		s.opts.Type = transport.TCP
	}
	s.listener = listener
	s.mu.Unlock()

	go s.waitForShutdown(ctx)
	go s.acceptLoop(ctx, listener, handler)

	return nil
}

func (s *Server) listenAddr() string {
	host := "0.0.0.0"
	if len(s.opts.Host) > 0 {
		host = s.opts.Host.String()
	}
	return fmt.Sprintf("%s:%d", host, s.opts.Port)
}

func (s *Server) getHandler() Handler {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.handler
}

func (s *Server) waitForShutdown(ctx context.Context) {
	<-ctx.Done()
	s.shutdown()
}

func (s *Server) shutdown() {
	s.once.Do(func() {
		close(s.closeCh)

		s.mu.Lock()
		listener := s.listener
		s.listener = nil
		conns := make([]net.Conn, 0, len(s.conns))
		for conn := range s.conns {
			conns = append(conns, conn)
		}
		s.mu.Unlock()

		if listener != nil {
			if err := listener.Close(); err != nil {
				log.Printf("tcp server close listener failed: %v\n", err)
			}
		}

		for _, conn := range conns {
			_ = conn.Close()
		}
	})
}

func (s *Server) acceptLoop(ctx context.Context, listener net.Listener, handler Handler) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			select {
			case <-ctx.Done():
				return
			case <-s.closeCh:
				return
			default:
			}

			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			log.Printf("tcp server accept failed: %v\n", err)
			return
		}

		s.trackConn(conn)
		go s.handleConn(ctx, conn, handler)
	}
}

func (s *Server) handleConn(ctx context.Context, conn net.Conn, handler Handler) {
	defer s.untrackConn(conn)
	defer conn.Close()

	if s.opts.IdleTimeout > 0 {
		_ = conn.SetDeadline(time.Now().Add(s.opts.IdleTimeout))
	}
	if s.opts.ReadTimeout > 0 {
		_ = conn.SetReadDeadline(time.Now().Add(s.opts.ReadTimeout))
	}
	if s.opts.WriteTimeout > 0 {
		_ = conn.SetWriteDeadline(time.Now().Add(s.opts.WriteTimeout))
	}

	handler(ctx, &connWithContext{Conn: conn, server: s})
	if err := ctx.Err(); err != nil && !errors.Is(err, context.Canceled) {
		log.Printf("tcp connection context ended with error: %v\n", err)
	}
}

func (s *Server) trackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.conns[conn] = struct{}{}
}

func (s *Server) untrackConn(conn net.Conn) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.conns, conn)
}

type connWithContext struct {
	net.Conn
	server *Server
}

func (c *connWithContext) Read(p []byte) (int, error) {
	if c.server.opts.ReadTimeout > 0 {
		_ = c.Conn.SetReadDeadline(time.Now().Add(c.server.opts.ReadTimeout))
	}
	return c.Conn.Read(p)
}

func (c *connWithContext) Write(p []byte) (int, error) {
	if c.server.opts.WriteTimeout > 0 {
		_ = c.Conn.SetWriteDeadline(time.Now().Add(c.server.opts.WriteTimeout))
	}
	return c.Conn.Write(p)
}
