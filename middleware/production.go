package middleware

import (
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

type CORSConfig struct {
	AllowedOrigins []string
	AllowedMethods []string
	AllowedHeaders []string
	MaxAge         time.Duration
}

func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("X-Frame-Options", "DENY")
		c.Header("Referrer-Policy", "no-referrer")
		c.Header("Content-Security-Policy", "default-src 'none'; frame-ancestors 'none'")
		c.Next()
	}
}

func CORS(cfg CORSConfig) gin.HandlerFunc {
	origins := make(map[string]struct{}, len(cfg.AllowedOrigins))
	for _, origin := range cfg.AllowedOrigins {
		origins[strings.TrimSpace(origin)] = struct{}{}
	}
	methods := strings.Join(cfg.AllowedMethods, ", ")
	if methods == "" {
		methods = "GET, POST, PUT, DELETE, OPTIONS"
	}
	headers := strings.Join(cfg.AllowedHeaders, ", ")
	if headers == "" {
		headers = "Authorization, Content-Type, X-Request-ID"
	}
	return func(c *gin.Context) {
		origin := c.GetHeader("Origin")
		if _, ok := origins[origin]; origin != "" && ok {
			c.Header("Access-Control-Allow-Origin", origin)
			c.Header("Vary", "Origin")
			c.Header("Access-Control-Allow-Methods", methods)
			c.Header("Access-Control-Allow-Headers", headers)
			if cfg.MaxAge > 0 {
				c.Header("Access-Control-Max-Age", strconv.FormatInt(int64(cfg.MaxAge/time.Second), 10))
			}
		}
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

// RateLimit applies a process-local token bucket. Distributed limits belong at
// the gateway; this protects individual instances from accidental overload.
func RateLimit(rate int, burst int) gin.HandlerFunc {
	if rate <= 0 {
		return func(c *gin.Context) { c.Next() }
	}
	if burst < rate {
		burst = rate
	}
	var mu sync.Mutex
	tokens, last := float64(burst), time.Now()
	return func(c *gin.Context) {
		mu.Lock()
		now := time.Now()
		tokens += now.Sub(last).Seconds() * float64(rate)
		if tokens > float64(burst) {
			tokens = float64(burst)
		}
		last = now
		allowed := tokens >= 1
		if allowed {
			tokens--
		}
		mu.Unlock()
		if !allowed {
			c.Header("Retry-After", "1")
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limit exceeded"})
			return
		}
		c.Next()
	}
}
