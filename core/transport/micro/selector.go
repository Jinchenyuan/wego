package micro

import (
	"errors"
	"fmt"
	"hash/fnv"
	"sync"
	"time"

	"go-micro.dev/v5/metadata"
	"go-micro.dev/v5/registry"
	"go-micro.dev/v5/registry/cache"
	"go-micro.dev/v5/selector"
)

type idSelector struct {
	so selector.Options
	rc cache.Cache
	mu sync.RWMutex
}

func (c *idSelector) newCache() cache.Cache {
	opts := make([]cache.Option, 0, 1)

	if c.so.Context != nil {
		if t, ok := c.so.Context.Value("selector_ttl").(time.Duration); ok {
			opts = append(opts, cache.WithTTL(t))
		}
	}

	return cache.New(c.so.Registry, opts...)
}

func (c *idSelector) Init(opts ...selector.Option) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, o := range opts {
		o(&c.so)
	}

	c.rc.Stop()
	c.rc = c.newCache()

	return nil
}

func (c *idSelector) Options() selector.Options {
	return c.so
}

func (c *idSelector) Select(service string, opts ...selector.SelectOption) (selector.Next, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	sopts := selector.SelectOptions{
		Strategy: c.so.Strategy,
	}

	for _, opt := range opts {
		opt(&sopts)
	}

	// get the service
	// try the cache first
	// if that fails go directly to the registry
	services, err := c.rc.GetService(service)
	if err != nil {
		if errors.Is(err, registry.ErrNotFound) {
			return nil, selector.ErrNotFound
		}

		return nil, err
	}

	var md metadata.Metadata
	if sopts.Context != nil {
		if m, ok := metadata.FromContext(sopts.Context); ok {
			md = m
			fmt.Printf("metadata: %+v\n", md)
		}
	}

	// apply the filters
	for _, filter := range sopts.Filters {
		services = filter(services)
	}

	// if there's nothing left, return
	if len(services) == 0 {
		return nil, selector.ErrNoneAvailable
	}

	var nodes []*registry.Node
	for _, svc := range services {
		nodes = append(nodes, svc.Nodes...)
	}

	for _, n := range nodes {
		// print node id
		fmt.Printf("node id: %s\n", n.Id)
	}

	if key := firstNonEmpty(md["uid"], md["Uid"], md["user-id"], md["x-uid"]); key != "" && len(nodes) > 0 {
		chosen := hrwPick(key, nodes)
		if chosen == nil {
			return nil, selector.ErrNoneAvailable
		}
		// Return a Next that always yields the chosen node (sticky mapping).
		return func() (*registry.Node, error) {
			return chosen, nil
		}, nil
	}

	// fallback to the original strategy
	return sopts.Strategy(services), nil
}

func (c *idSelector) Mark(service string, node *registry.Node, err error) {
}

func (c *idSelector) Reset(service string) {
}

// Close stops the watcher and destroys the cache.
func (c *idSelector) Close() error {
	c.rc.Stop()

	return nil
}

func (c *idSelector) String() string {
	return "idRegistry"
}

func hrwPick(key string, nodes []*registry.Node) *registry.Node {
	var best *registry.Node
	var bestScore uint64
	for _, n := range nodes {
		// score = FNV64a(key + "\x00" + nodeID)
		h := fnv.New64a()
		_, _ = h.Write([]byte(key))
		_, _ = h.Write([]byte{0})
		_, _ = h.Write([]byte(n.Id))
		score := h.Sum64()
		if best == nil || score > bestScore {
			best = n
			bestScore = score
		}
	}
	return best
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// ...existing code...

func newSelector(opts ...selector.Option) selector.Selector {
	sopts := selector.Options{
		Strategy: selector.Random,
	}

	for _, opt := range opts {
		opt(&sopts)
	}

	if sopts.Registry == nil {
		sopts.Registry = registry.DefaultRegistry
	}

	s := &idSelector{
		so: sopts,
	}
	s.rc = s.newCache()

	return s
}

func NewSelectorDependId(reg registry.Registry) selector.Selector {
	return newSelector(
		selector.SetStrategy(selector.Random),
		selector.Registry(reg),
	)
}
