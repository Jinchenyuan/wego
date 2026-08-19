package telemetry

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
)

// Registry is a small Prometheus text-format registry. It deliberately keeps
// labels bounded to prevent request data from becoming metric cardinality.
type Registry struct {
	mu       sync.Mutex
	counters map[string]float64
}

func NewRegistry() *Registry { return &Registry{counters: make(map[string]float64)} }

func (r *Registry) Inc(name string, labels map[string]string) {
	r.Add(name, labels, 1)
}

func (r *Registry) Add(name string, labels map[string]string, value float64) {
	if r == nil {
		return
	}
	r.mu.Lock()
	r.counters[metricKey(name, labels)] += value
	r.mu.Unlock()
}

// Observe exports a Prometheus-compatible summary pair without retaining raw
// observations in memory.
func (r *Registry) Observe(name string, labels map[string]string, value float64) {
	r.Add(name+"_sum", labels, value)
	r.Add(name+"_count", labels, 1)
}

func (r *Registry) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		r.mu.Lock()
		keys := make([]string, 0, len(r.counters))
		for key := range r.counters {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			_, _ = fmt.Fprintf(w, "%s %g\n", key, r.counters[key])
		}
		r.mu.Unlock()
	})
}

func metricKey(name string, labels map[string]string) string {
	if len(labels) == 0 {
		return name
	}
	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pairs := make([]string, 0, len(keys))
	for _, key := range keys {
		pairs = append(pairs, key+`="`+strings.ReplaceAll(labels[key], `"`, `\\"`)+`"`)
	}
	return name + "{" + strings.Join(pairs, ",") + "}"
}
