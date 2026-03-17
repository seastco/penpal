package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/google/uuid"
	"github.com/stove/penpal/internal/client"
	pencrypto "github.com/stove/penpal/internal/crypto"
)

var version = "dev"

func main() {
	if len(os.Args) == 2 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println("penpal " + version)
		return
	}

	serverURL := envOr("PENPAL_SERVER", "wss://getpenpal.dev")

	// Warn if connecting to a remote server without TLS
	if strings.HasPrefix(serverURL, "ws://") && !strings.Contains(serverURL, "localhost") && !strings.Contains(serverURL, "127.0.0.1") {
		fmt.Fprintf(os.Stderr, "WARNING: connecting to %s without TLS — auth signatures and encrypted messages may be intercepted in transit. Use wss:// for production.\n", serverURL)
	}

	app := &client.AppState{
		ServerURL: serverURL,
		Network:   client.NewNetwork(serverURL),
	}

	// Check for existing account
	if pencrypto.KeyFileExists() {
		pub, priv, err := pencrypto.LoadKeyFile()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading key: %v\n", err)
			os.Exit(1)
		}
		app.PublicKey = pub
		app.PrivateKey = priv

		username, disc, err := client.LoadIdentityPublic()
		if err != nil {
			fmt.Fprintf(os.Stderr, "error loading identity: %v\n", err)
			os.Exit(1)
		}
		app.Username = username
		app.Discriminator = disc
	}

	// Connect to server
	ctx := context.Background()
	if err := app.Network.Connect(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "cannot connect to server: %v\n", err)
		fmt.Fprintf(os.Stderr, "make sure the penpal server is running at %s\n", serverURL)
		os.Exit(1)
	}
	defer app.Network.Close()

	// If we have keys, try to authenticate (if it fails, fall through to registration)
	if app.PrivateKey != nil && app.Username != "" {
		authResp, err := app.Network.Authenticate(ctx, app.Username, app.Discriminator, app.PrivateKey)
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: auth failed for %s#%s: %v\n", app.Username, app.Discriminator, err)
			fmt.Fprintf(os.Stderr, "starting fresh — you can re-register or recover your account\n")
			// Clear stale identity so TUI shows registration
			app.Username = ""
			app.Discriminator = ""
			app.PublicKey = nil
			app.PrivateKey = nil
		} else {
			app.UserID = authResp.User.ID
			app.HomeCity = authResp.User.HomeCity
		}
	}

	// Create glamour renderer BEFORE bubbletea starts reading stdin.
	// glamour.WithAutoStyle() queries the terminal for background color via
	// OSC escape sequences. Once bubbletea's input goroutine is running, it
	// races for the terminal response and can cause 5-second timeouts.
	renderer, _ := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(60),
	)
	app.GlamourRenderer = renderer
	app.DecryptedBodies = make(map[uuid.UUID]string)

	// Run TUI
	tui := client.NewTUI(app)
	p := tea.NewProgram(tui, tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
