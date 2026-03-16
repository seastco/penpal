package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/stove/penpal/internal/server"
)

func main() {
	cfg := server.Config{
		ListenAddr: envOr("PENPAL_LISTEN", ":8282"),
		DBConnStr:  envOr("PENPAL_DB", "postgres://localhost:5432/penpal?sslmode=disable"),
		CityGraph:  envOr("PENPAL_CITIES", "data/graph.json"),
	}

	srv, err := server.New(cfg)
	if err != nil {
		log.Fatalf("failed to create server: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down...")
		cancel()
		srv.Shutdown(context.Background())
	}()

	if err := srv.Start(ctx); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
