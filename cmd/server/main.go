package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "time/tzdata" // embed timezone database for systems without it

	"github.com/stove/penpal/internal/server"
)

func main() {
	cfg := server.Config{
		ListenAddr: envOr("PENPAL_LISTEN", ":8282"),
		DBConnStr:  envOr("PENPAL_DB", "postgres://localhost:5432/penpal?sslmode=disable"),
		CityGraph:  envOr("PENPAL_CITIES", "data/graph.json"),
		TrustProxy: os.Getenv("PENPAL_TRUST_PROXY") == "true",
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
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		srv.Shutdown(shutdownCtx)
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
