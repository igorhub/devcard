package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/igorhub/devcard/pkg/server"
)

func run(cfg server.Config) (restart bool) {
	restartC := make(chan struct{})
	server := server.NewServer(cfg, restartC)
	httpServer := http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Port),
		Handler: server,
	}

	ctx := context.Background()
	ctx, cancel := signal.NotifyContext(ctx, os.Interrupt)
	defer cancel()

	go func() {
		log.Printf("Starting devcards...")
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("Error running httpServer.ListenAndServe:", err)
			os.Exit(1)
		}
		log.Printf("Access the app via the following URL: http://127.0.0.1:%d\n", cfg.Port)
	}()

	done := make(chan struct{})
	go func() {
		select {
		case <-restartC:
			restart = true
		case <-ctx.Done():
		}
		log.Println("Shutting down the server...")
		server.Shutdown()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := httpServer.Shutdown(shutdownCtx); err != nil {
			log.Println("Error running httpServer.Shutdown:", err)
		}
		done <- struct{}{}
	}()
	<-done
	return restart
}

func main() {
	for {
		cfg := server.LoadConfig()
		restart := run(cfg)
		if !restart {
			break
		}
	}
}
