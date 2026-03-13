package reminder

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
)

var ErrNoNotifierHandler = errors.New("no reminder notifier registered")

type PayloadEnvelope struct {
	Type string `json:"type"`
}

type DispatcherNotifier struct {
	mu              sync.RWMutex
	routeHandlers   map[dispatchRoute]Notifier
	typeHandlers    map[string]Notifier
	channelHandlers map[string]Notifier
	fallback        Notifier
}

type dispatchRoute struct {
	channel string
	typeKey string
}

func NewDispatcherNotifier() *DispatcherNotifier {
	return &DispatcherNotifier{
		routeHandlers:   make(map[dispatchRoute]Notifier),
		typeHandlers:    make(map[string]Notifier),
		channelHandlers: make(map[string]Notifier),
	}
}

func (d *DispatcherNotifier) Register(channel string, typeKey string, handler Notifier) error {
	if d == nil {
		return ErrNoNotifierHandler
	}
	if handler == nil {
		return errors.New("reminder handler is nil")
	}

	key := dispatchRoute{
		channel: normalizeDispatchKey(channel),
		typeKey: normalizeDispatchKey(typeKey),
	}
	if key.channel == "" || key.typeKey == "" {
		return errors.New("channel and type are required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.routeHandlers[key] = handler
	return nil
}

func (d *DispatcherNotifier) RegisterChannel(channel string, handler Notifier) error {
	if d == nil {
		return ErrNoNotifierHandler
	}
	if handler == nil {
		return errors.New("reminder handler is nil")
	}

	channel = normalizeDispatchKey(channel)
	if channel == "" {
		return errors.New("channel is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.channelHandlers[channel] = handler
	return nil
}

func (d *DispatcherNotifier) RegisterType(typeKey string, handler Notifier) error {
	if d == nil {
		return ErrNoNotifierHandler
	}
	if handler == nil {
		return errors.New("reminder handler is nil")
	}

	typeKey = normalizeDispatchKey(typeKey)
	if typeKey == "" {
		return errors.New("type is required")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.typeHandlers[typeKey] = handler
	return nil
}

func (d *DispatcherNotifier) SetFallback(handler Notifier) error {
	if d == nil {
		return ErrNoNotifierHandler
	}
	if handler == nil {
		return errors.New("reminder handler is nil")
	}

	d.mu.Lock()
	defer d.mu.Unlock()
	d.fallback = handler
	return nil
}

func (d *DispatcherNotifier) Notify(ctx context.Context, reminder *Reminder) error {
	if d == nil {
		return ErrNoNotifierHandler
	}
	if reminder == nil {
		return ErrInvalidReminder
	}

	channel := normalizeDispatchKey(reminder.Channel)
	typeKey, payloadErr := payloadType(reminder.Payload)

	handler, found := d.lookup(channel, typeKey)
	if found {
		return handler.Notify(ctx, reminder)
	}

	if payloadErr != nil {
		return fmt.Errorf("decode reminder payload: %w", payloadErr)
	}

	return ErrNoNotifierHandler
}

func (d *DispatcherNotifier) lookup(channel string, typeKey string) (Notifier, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()

	if channel != "" && typeKey != "" {
		if handler, ok := d.routeHandlers[dispatchRoute{channel: channel, typeKey: typeKey}]; ok {
			return handler, true
		}
	}
	if typeKey != "" {
		if handler, ok := d.typeHandlers[typeKey]; ok {
			return handler, true
		}
	}
	if channel != "" {
		if handler, ok := d.channelHandlers[channel]; ok {
			return handler, true
		}
	}
	if d.fallback != nil {
		return d.fallback, true
	}

	return nil, false
}

func payloadType(payload string) (string, error) {
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return "", nil
	}

	var envelope PayloadEnvelope
	if err := json.Unmarshal([]byte(payload), &envelope); err != nil {
		return "", err
	}

	return normalizeDispatchKey(envelope.Type), nil
}

func normalizeDispatchKey(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
