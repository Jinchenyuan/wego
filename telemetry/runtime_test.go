package telemetry

import (
	"context"
	"testing"
)

func TestDisabledRuntimeIsNoopAndClosable(t *testing.T) {
	runtime, err := New(context.Background(), Config{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	ctx, span := runtime.Tracer("test").Start(context.Background(), "operation")
	if ctx == nil || span == nil {
		t.Fatal("expected usable no-op tracer")
	}
	span.End()
	if err := runtime.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}

func TestEnabledRuntimeValidatesEndpoint(t *testing.T) {
	if _, err := New(context.Background(), Config{Enabled: true, ServiceName: "orders"}); err == nil {
		t.Fatal("expected missing endpoint error")
	}
}
