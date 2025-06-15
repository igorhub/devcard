package server

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/igorhub/devcard/pkg/internal/config"
)

func Run(port int) error {
	cfg := config.LoadConfig()
	if port != 0 {
		cfg.Port = port
	}
	return run(cfg)
}

func run(cfg config.Config) error {
	server := NewServer(cfg)
	httpServer := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: server,
	}

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	var serverError error
	go func() {
		log.Printf("Starting devcards...")
		log.Printf("Access the app via the following URL: http://127.0.0.1:%d\n", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("Error running httpServer.ListenAndServe:", err)
			serverError = err
			cancel()
		}
	}()

	done := make(chan struct{})
	go func() {
		<-ctx.Done()
		log.Println("Shutting down the server...")
		server.Shutdown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Println("Error running httpServer.Shutdown:", err)
		}
		close(done)
	}()

	<-done
	return serverError
}
