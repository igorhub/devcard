package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"time"

	"github.com/igorhub/devcard/pkg/server"
)

const version = "v0.9.0"

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
		log.Printf("Access the app via the following URL: http://127.0.0.1:%d\n", cfg.Port)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Println("Error running httpServer.ListenAndServe:", err)
			os.Exit(1)
		}
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
		close(done)
	}()
	<-done
	return restart
}

func main() {
	var port int
	var showVersion bool
	flag.IntVar(&port, "port", 0, "Port for the devcards server")
	flag.BoolVar(&showVersion, "version", false, "Show version")
	flag.Parse()

	if showVersion {
		fmt.Println(version)
		os.Exit(0)
	}

	for {
		cfg := server.LoadConfig()
		if port != 0 {
			cfg.Port = port
		}
		restart := run(cfg)
		if !restart {
			break
		}
	}
}
