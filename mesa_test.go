package wego

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
)

type testComponent struct {
	started atomic.Bool
}

func (t *testComponent) Start(context.Context) error {
	t.started.Store(true)
	return nil
}

func (t *testComponent) Name() string {
	return "test-component"
}

func TestRegisterComponent(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	if len(mesa.components) != 1 {
		t.Fatalf("expected 1 component, got %d", len(mesa.components))
	}
	if mesa.components[0] != component {
		t.Fatalf("expected stored component to match input")
	}
}

func TestRegisterComponentAfterRuntimeStarted(t *testing.T) {
	mesa := &Mesa{runtimeStarted: true, componentIndex: make(map[string]Component)}

	err := mesa.RegisterComponent(&testComponent{})
	if err == nil {
		t.Fatalf("expected registration to fail after runtime start")
	}
	if err != ErrRuntimeStarted {
		t.Fatalf("expected ErrRuntimeStarted, got %v", err)
	}
}

func TestGetComponent(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	got, ok := mesa.GetComponent("test-component")
	if !ok {
		t.Fatalf("expected component lookup to succeed")
	}
	if got != component {
		t.Fatalf("expected looked up component to match input")
	}
}

func TestRegisterComponentRejectsDuplicateName(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}

	if err := mesa.RegisterComponent(&testComponent{}); err != nil {
		t.Fatalf("expected first registration to succeed, got %v", err)
	}

	err := mesa.RegisterComponent(&testComponent{})
	if err == nil {
		t.Fatalf("expected duplicate registration to fail")
	}
	if !errors.Is(err, ErrComponentExists) {
		t.Fatalf("expected ErrComponentExists, got %v", err)
	}
}

func TestGetComponentAs(t *testing.T) {
	mesa := &Mesa{componentIndex: make(map[string]Component)}
	component := &testComponent{}

	if err := mesa.RegisterComponent(component); err != nil {
		t.Fatalf("expected component registration to succeed, got %v", err)
	}

	typed, ok := GetComponentAs[*testComponent](mesa, "test-component")
	if !ok {
		t.Fatalf("expected typed lookup to succeed")
	}
	if typed != component {
		t.Fatalf("expected typed component to match input")
	}
}
