package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	_ "time/tzdata" // embed timezone database for systems without it

	"github.com/stove/penpal/internal/db"
	"github.com/stove/penpal/internal/models"
	"github.com/stove/penpal/internal/server"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "mint" {
		if err := runMint(os.Args[2:]); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		return
	}

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

func runMint(args []string) error {
	if len(args) == 0 {
		printMintUsage()
		return nil
	}

	connStr := envOr("PENPAL_DB", "postgres://localhost:5432/penpal?sslmode=disable")
	database, err := db.New(connStr)
	if err != nil {
		return fmt.Errorf("connecting to database: %w", err)
	}

	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	switch args[0] {
	case "--list-users":
		users, err := database.GetAllUsers(ctx)
		if err != nil {
			return err
		}
		for _, u := range users {
			fmt.Printf("%s#%s\n", u.Username, u.Discriminator)
		}
		return nil

	case "--list-stamps":
		types := make([]string, 0, len(models.ValidStampTypes))
		for t := range models.ValidStampTypes {
			types = append(types, t)
		}
		sort.Strings(types)
		for _, t := range types {
			fmt.Printf("%-24s %s\n", t, models.ValidStampTypes[t])
		}
		return nil

	case "--help", "-h":
		printMintUsage()
		return nil
	}

	// Parse user#disc
	address := args[0]
	parts := strings.SplitN(address, "#", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return fmt.Errorf("invalid address %q — expected user#disc", address)
	}

	user, err := database.GetUserByAddress(ctx, parts[0], parts[1])
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user %s not found", address)
	}

	stampTypes := args[1:]
	if len(stampTypes) == 0 {
		return fmt.Errorf("no stamp types specified")
	}

	for _, st := range stampTypes {
		rarity, ok := models.ValidStampTypes[st]
		if !ok {
			return fmt.Errorf("unknown stamp type %q — use --list-stamps to see valid types", st)
		}

		stamp, err := database.CreateStamp(ctx, user.ID, st, rarity, models.EarnedMint)
		if err != nil {
			return fmt.Errorf("minting %q: %w", st, err)
		}
		fmt.Printf("Minted %q (%s) for %s — id=%s\n", st, rarity, address, stamp.ID)
	}

	return nil
}

func printMintUsage() {
	fmt.Print(`Usage:
  penpal-server mint <user#disc> <stamp> [stamp ...]
  penpal-server mint --list-users
  penpal-server mint --list-stamps

Examples:
  penpal-server mint alice#1234 state:ca rare:explorer
  penpal-server mint bob#5678 common:star common:star common:star
`)
}
