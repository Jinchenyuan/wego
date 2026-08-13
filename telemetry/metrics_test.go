package telemetry

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRegistryPrometheusOutput(t *testing.T) {
	r := NewRegistry()
	r.Inc("wego_requests_total", map[string]string{"method": "GET", "route": "/livez"})
	r.Inc("wego_requests_total", map[string]string{"route": "/livez", "method": "GET"})
	w := httptest.NewRecorder()
	r.Handler().ServeHTTP(w, httptest.NewRequest("GET", "/metrics", nil))
	if got := w.Body.String(); !strings.Contains(got, `wego_requests_total{method="GET",route="/livez"} 2`) {
		t.Fatalf("metrics = %q", got)
	}
}
