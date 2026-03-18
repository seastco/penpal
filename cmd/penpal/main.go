package main

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	_ "time/tzdata" // embed timezone database for systems without it

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/glamour"
	"github.com/google/uuid"
	"github.com/stove/penpal/internal/client"
	pencrypto "github.com/stove/penpal/internal/crypto"
)

var version = "dev"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "--version", "-v":
			fmt.Println("penpal " + version)
			return
		case "--help", "-h":
			printUsage()
			return
		case "update":
			if err := runUpdate(); err != nil {
				fmt.Fprintf(os.Stderr, "error: %v\n", err)
				os.Exit(1)
			}
			return
		default:
			fmt.Fprintf(os.Stderr, "unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
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
	//
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

func printUsage() {
	fmt.Print(`penpal - send letters that take real time to travel

Usage:
  penpal              Start the TUI
  penpal update       Update to the latest version
  penpal --version    Print version
  penpal --help       Show this help

Environment:
  PENPAL_SERVER       Server URL (default: wss://getpenpal.dev)
  PENPAL_HOME         Config directory (default: ~/.penpal)
`)
}

const repo = "seastco/penpal"

func runUpdate() error {
	fmt.Println("Checking for updates...")

	// Fetch latest release from GitHub
	resp, err := http.Get("https://api.github.com/repos/" + repo + "/releases/latest")
	if err != nil {
		return fmt.Errorf("could not check for updates: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		return fmt.Errorf("could not check for updates (HTTP %d)", resp.StatusCode)
	}

	var release struct {
		TagName string `json:"tag_name"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&release); err != nil {
		return fmt.Errorf("could not parse release info: %w", err)
	}

	latest := strings.TrimPrefix(release.TagName, "v")

	if version != "dev" && version == latest {
		fmt.Printf("Already up to date (v%s).\n", version)
		return nil
	}

	if version == "dev" {
		fmt.Println("Running a dev build — updating to latest release...")
	} else {
		fmt.Printf("Updating penpal: v%s → v%s\n", version, latest)
	}

	// Download archive
	archive := fmt.Sprintf("penpal-%s-%s.tar.gz", runtime.GOOS, runtime.GOARCH)
	url := fmt.Sprintf("https://github.com/%s/releases/download/%s/%s", repo, release.TagName, archive)

	dlResp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("could not download update: %w", err)
	}
	defer dlResp.Body.Close()

	if dlResp.StatusCode != 200 {
		return fmt.Errorf("no release found for %s/%s (HTTP %d)", runtime.GOOS, runtime.GOARCH, dlResp.StatusCode)
	}

	// Extract binary from tar.gz
	binary, err := extractBinary(dlResp.Body)
	if err != nil {
		return fmt.Errorf("could not extract update: %w", err)
	}

	// Find current executable path
	execPath, err := os.Executable()
	if err != nil {
		return fmt.Errorf("could not find current executable: %w", err)
	}
	execPath, err = filepath.EvalSymlinks(execPath)
	if err != nil {
		return fmt.Errorf("could not resolve executable path: %w", err)
	}

	// Write to temp file in the same directory (for atomic rename)
	dir := filepath.Dir(execPath)
	tmp, err := os.CreateTemp(dir, "penpal-update-*")
	if err != nil {
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("permission denied — try: sudo penpal update")
		}
		return fmt.Errorf("could not create temp file: %w", err)
	}
	tmpPath := tmp.Name()

	if _, err := tmp.Write(binary); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("could not write update: %w", err)
	}
	if err := tmp.Chmod(0755); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("could not set permissions: %w", err)
	}
	tmp.Close()

	// Atomic replace
	if err := os.Rename(tmpPath, execPath); err != nil {
		os.Remove(tmpPath)
		if errors.Is(err, os.ErrPermission) {
			return fmt.Errorf("permission denied — try: sudo penpal update")
		}
		return fmt.Errorf("could not replace binary: %w", err)
	}

	fmt.Printf("Updated to v%s.\n", latest)
	return nil
}

func extractBinary(r io.Reader) ([]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	defer gz.Close()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return nil, fmt.Errorf("penpal binary not found in archive")
		}
		if err != nil {
			return nil, err
		}
		if filepath.Base(hdr.Name) == "penpal" && hdr.Typeflag == tar.TypeReg {
			return io.ReadAll(tr)
		}
	}
}
