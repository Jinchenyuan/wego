package tcp

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/Jinchenyuan/wego/transport"
)

func TestServerStartRequiresHandler(t *testing.T) {
	t.Parallel()

	srv := NewTCPServer(WithPort(0))
	if err := srv.Start(context.Background()); err != ErrHandlerRequired {
		t.Fatalf("expected ErrHandlerRequired, got %v", err)
	}
}

func TestServerStartAndHandleConnection(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	received := make(chan string, 1)
	srv := NewTCPServer(
		WithHost(net.ParseIP("127.0.0.1")),
		WithPort(0),
		WithType(transport.TCP),
		WithHandler(func(ctx context.Context, conn net.Conn) {
			reader := bufio.NewReader(conn)
			line, err := reader.ReadString('\n')
			if err != nil {
				return
			}
			received <- line
			_, _ = conn.Write([]byte(fmt.Sprintf("ack:%s", line)))
		}),
	)

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start tcp server: %v", err)
	}

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("expected listener addr")
	}

	conn, err := net.DialTimeout("tcp", addr.String(), time.Second)
	if err != nil {
		t.Fatalf("dial tcp server: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Write([]byte("ping\n")); err != nil {
		t.Fatalf("write request: %v", err)
	}

	buf := make([]byte, 64)
	n, err := conn.Read(buf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}

	if got := string(buf[:n]); got != "ack:ping\n" {
		t.Fatalf("unexpected response: %q", got)
	}

	select {
	case got := <-received:
		if got != "ping\n" {
			t.Fatalf("unexpected request payload: %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for tcp handler")
	}
}

func TestServerShutdownClosesListener(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	srv := NewTCPServer(
		WithHost(net.ParseIP("127.0.0.1")),
		WithPort(0),
		WithHandler(func(ctx context.Context, conn net.Conn) {
			<-ctx.Done()
		}),
	)

	if err := srv.Start(ctx); err != nil {
		t.Fatalf("start tcp server: %v", err)
	}

	addr := srv.Addr()
	if addr == nil {
		t.Fatal("expected listener addr")
	}

	cancel()
	t.Cleanup(cancel)

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr.String(), 50*time.Millisecond)
		if err != nil {
			return
		}
		_ = conn.Close()
		time.Sleep(20 * time.Millisecond)
	}

	t.Fatal("tcp listener still accepted connections after shutdown")
}
