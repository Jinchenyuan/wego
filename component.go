package wego

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

type Component interface {
	Start(context.Context) error
	Name() string
}

var ErrComponentNotFound = errors.New("component not found")
var ErrComponentExists = errors.New("component already registered")

func normalizeComponentName(name string) string {
	return strings.TrimSpace(name)
}

func GetComponentAs[T any](m *Mesa, name string) (T, bool) {
	var zero T
	if m == nil {
		return zero, false
	}

	component, ok := m.GetComponent(name)
	if !ok {
		return zero, false
	}

	typed, ok := component.(T)
	if !ok {
		return zero, false
	}

	return typed, true
}

func MustGetComponentAs[T any](m *Mesa, name string) T {
	component, ok := GetComponentAs[T](m, name)
	if ok {
		return component
	}

	panic(fmt.Sprintf("component %q not found or has unexpected type", name))
}
