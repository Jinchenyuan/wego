package http

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Jinchenyuan/wego/telemetry"
	"github.com/gin-gonic/gin"
)

func TestProductionDefaultsAndRequestID(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewHTTPServer()
	if s.ReadHeaderTimeout == 0 || s.ReadTimeout == 0 || s.WriteTimeout == 0 || s.IdleTimeout == 0 || s.MaxHeaderBytes == 0 {
		t.Fatal("unsafe HTTP defaults")
	}
	s.GetEngine().GET("/test", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, httptest.NewRequest("GET", "/test", nil))
	if w.Header().Get("X-Request-ID") == "" {
		t.Fatal("missing request id")
	}
}

func TestTimeoutOverrides(t *testing.T) {
	s := NewHTTPServer(WithTimeouts(time.Second, 2*time.Second, 3*time.Second, 4*time.Second))
	if s.ReadHeaderTimeout != time.Second || s.IdleTimeout != 4*time.Second {
		t.Fatal("timeout overrides not applied")
	}
}

func TestRegisterRouteWithoutAuth(t *testing.T) {
	gin.SetMode(gin.TestMode)
	s := NewHTTPServer()
	s.RegisterRoute(http.MethodGet, "/public", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/public", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestTelemetryMiddlewareRecordsRequest(t *testing.T) {
	runtime, err := telemetry.New(nil, telemetry.Config{})
	if err != nil {
		t.Fatalf("telemetry.New: %v", err)
	}
	metrics := telemetry.NewRegistry()
	s := NewHTTPServer(WithTelemetry(runtime, metrics))
	s.GetEngine().GET("/users/:id", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	w := httptest.NewRecorder()
	s.Handler.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/users/42", nil))
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d", w.Code)
	}
	metricsResponse := httptest.NewRecorder()
	metrics.Handler().ServeHTTP(metricsResponse, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if !strings.Contains(metricsResponse.Body.String(), `route="/users/:id"`) {
		t.Fatalf("metrics = %q", metricsResponse.Body.String())
	}
}
