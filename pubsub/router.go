package pubsub

import (
	"fmt"
	"strings"
	"sync"
)

type Subscription struct {
	Topic       string
	Group       string
	Consumer    string
	Concurrency int
	Handler     Handler
	Retry       RetryPolicy
}

func (s Subscription) Validate() error {
	if normalizeTopic(s.Topic) == "" || normalizeGroup(s.Group) == "" {
		return ErrInvalidSubscription
	}
	if s.Handler == nil {
		return ErrHandlerRequired
	}
	return nil
}

type Router struct {
	mu      sync.RWMutex
	started bool
	subs    map[string]Subscription
}

func NewRouter() *Router {
	return &Router{subs: make(map[string]Subscription)}
}

func (r *Router) Add(sub Subscription) error {
	if err := sub.Validate(); err != nil {
		return err
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if r.started {
		return ErrServiceStarted
	}
	key := subscriptionKey(sub.Topic, sub.Group)
	if _, exists := r.subs[key]; exists {
		return fmt.Errorf("%w: %s", ErrDuplicateSubscription, key)
	}
	if sub.Concurrency <= 0 {
		sub.Concurrency = 1
	}
	if strings.TrimSpace(sub.Consumer) == "" {
		sub.Consumer = ""
	}
	sub.Topic = normalizeTopic(sub.Topic)
	sub.Group = normalizeGroup(sub.Group)
	r.subs[key] = sub
	return nil
}

func (r *Router) List() []Subscription {
	r.mu.RLock()
	defer r.mu.RUnlock()
	items := make([]Subscription, 0, len(r.subs))
	for _, sub := range r.subs {
		items = append(items, sub)
	}
	return items
}

func (r *Router) Freeze() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.started = true
}

func subscriptionKey(topic string, group string) string {
	return normalizeTopic(topic) + "::" + normalizeGroup(group)
}

func normalizeTopic(topic string) string {
	return strings.TrimSpace(strings.ToLower(topic))
}

func normalizeGroup(group string) string {
	return strings.TrimSpace(strings.ToLower(group))
}
