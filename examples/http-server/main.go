package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/Jinchenyuan/wego"
	"github.com/Jinchenyuan/wego/transport"
	httptransport "github.com/Jinchenyuan/wego/transport/http"
	"github.com/gin-gonic/gin"
)

func main() {
	port, err := httpPort()
	if err != nil {
		log.Fatal(err)
	}

	mesa, err := wego.New(wego.WithHTTPConfig(wego.HTTPConfig{
		Port:              port,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       60 * time.Second,
		MaxHeaderBytes:    1 << 20,
		MaxBodyBytes:      1 << 20,
		RequestTimeout:    10 * time.Second,
	}))
	if err != nil {
		log.Fatal(err)
	}

	server, ok := mesa.GetServerByType(transport.HTTP).(*httptransport.Server)
	if !ok {
		log.Fatal("HTTP server is not configured")
	}
	server.GetEngine().GET("/hello", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "hello from Wego"})
	})

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := mesa.Run(ctx); err != nil {
		log.Fatal(err)
	}
}

func httpPort() (int, error) {
	const defaultPort = 8080
	raw := os.Getenv("WEGO_HTTP_PORT")
	if raw == "" {
		return defaultPort, nil
	}

	port, err := strconv.Atoi(raw)
	if err != nil || port < 1 || port > 65535 {
		return 0, fmt.Errorf("invalid WEGO_HTTP_PORT %q: must be an integer from 1 to 65535", raw)
	}
	return port, nil
}
