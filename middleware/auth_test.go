package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestAuthenticateStoresPrincipal(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Authenticate(AuthenticatorFunc(func(_ context.Context, credential string) (Principal, error) {
		if credential != "valid-token" {
			return Principal{}, ErrUnauthenticated
		}
		return Principal{Subject: "user-1"}, nil
	})))
	router.GET("/private", func(c *gin.Context) {
		principal, ok := PrincipalFromContext(c.Request.Context())
		if !ok {
			c.Status(http.StatusInternalServerError)
			return
		}
		c.String(http.StatusOK, principal.Subject)
	})

	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusOK || w.Body.String() != "user-1" {
		t.Fatalf("unexpected response: status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestAuthenticateRejectsInvalidCredential(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(Authenticate(AuthenticatorFunc(func(context.Context, string) (Principal, error) {
		return Principal{}, errors.New("rejected")
	})))
	router.GET("/private", func(c *gin.Context) { c.Status(http.StatusNoContent) })

	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/private", nil))
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestLegacyAuthMiddleware(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	router.Use(AuthMiddleware("X-User-ID", func(id string) string {
		if id == "user-1" {
			return "valid-token"
		}
		return ""
	}))
	router.GET("/private", func(c *gin.Context) { c.Status(http.StatusNoContent) })
	req := httptest.NewRequest(http.MethodGet, "/private", nil)
	req.Header.Set("X-User-ID", "user-1")
	req.Header.Set("Authorization", "Bearer valid-token")
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}
