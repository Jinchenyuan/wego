package middleware

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

var ErrUnauthenticated = errors.New("unauthenticated")

const principalContextKey = "wego.principal"

type Principal struct {
	Subject    string
	Attributes map[string]string
}

type Authenticator interface {
	Authenticate(context.Context, string) (Principal, error)
}

type AuthenticatorFunc func(context.Context, string) (Principal, error)

func (f AuthenticatorFunc) Authenticate(ctx context.Context, credential string) (Principal, error) {
	return f(ctx, credential)
}

func PrincipalFromContext(ctx context.Context) (Principal, bool) {
	principal, ok := ctx.Value(principalContextKey).(Principal)
	return principal, ok
}

func Authenticate(authenticator Authenticator, excludePaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if slices.Contains(excludePaths, c.FullPath()) {
			c.Next()
			return
		}
		if authenticator == nil {
			abortUnauthorized(c, "authentication is not configured")
			return
		}
		credential, ok := bearerCredential(c.GetHeader("Authorization"))
		if !ok {
			abortUnauthorized(c, "invalid authorization header")
			return
		}
		principal, err := authenticator.Authenticate(c.Request.Context(), credential)
		if err != nil || strings.TrimSpace(principal.Subject) == "" {
			abortUnauthorized(c, "invalid credentials")
			return
		}
		ctx := context.WithValue(c.Request.Context(), principalContextKey, principal)
		c.Request = c.Request.WithContext(ctx)
		c.Next()
	}
}

func AuthMiddleware(idKey string, getCacheToken func(id string) string, excludePaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if slices.Contains(excludePaths, c.FullPath()) {
			c.Next()
			return
		}
		token, ok := bearerCredential(c.GetHeader("Authorization"))
		if !ok || getCacheToken == nil {
			abortUnauthorized(c, "invalid authorization header")
			return
		}
		id := strings.TrimSpace(c.GetHeader(idKey))
		cachedToken := getCacheToken(id)
		if id == "" || cachedToken == "" || cachedToken != token {
			abortUnauthorized(c, "invalid token")
			return
		}
		c.Next()
	}
}

func bearerCredential(header string) (string, bool) {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return "", false
	}
	credential := strings.TrimSpace(strings.TrimPrefix(header, prefix))
	return credential, credential != ""
}

func abortUnauthorized(c *gin.Context, message string) {
	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": message})
}
