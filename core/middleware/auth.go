package middleware

import (
	"net/http"
	"slices"
	"strings"

	"github.com/gin-gonic/gin"
)

func AuthMiddleware(idKey string, getCacheToken func(id string) string, excludePaths ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		if slices.Contains(excludePaths, c.FullPath()) {
			c.Next()
			return
		}

		auth := c.GetHeader("Authorization")
		if auth == "" {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "missing authorization header"})
			c.Abort()
			return
		}

		if !strings.HasPrefix(auth, "Bearer ") {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid authorization header"})
			c.Abort()
			return
		}

		token := strings.TrimPrefix(auth, "Bearer ")
		id := c.GetHeader(idKey)
		cachedTokenStr := getCacheToken(id)
		if cachedTokenStr == "" || cachedTokenStr != token {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid token"})
			c.Abort()
			return
		}

		c.Next()
	}
}
