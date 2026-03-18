package client

import (
	"crypto/ed25519"

	"github.com/charmbracelet/glamour"
	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/models"
	"github.com/seastco/penpal/internal/protocol"
)

// AppState holds the shared application state for the TUI.
type AppState struct {
	// Connection
	Network *Network

	// User identity
	UserID        uuid.UUID
	Username      string
	Discriminator string
	PublicKey     ed25519.PublicKey
	PrivateKey    ed25519.PrivateKey
	HomeCity      string

	// Cached data
	Contacts  []protocol.ContactItem
	Inbox     []protocol.InboxItem
	Sent      []protocol.SentItem
	InTransit []protocol.InTransitItem
	Stamps    []models.Stamp

	// Rendering
	GlamourRenderer *glamour.TermRenderer
	DecryptedBodies map[uuid.UUID]string

	// Server URL
	ServerURL string

	// Theme
	ThemeName string
}

// Address returns the full user address.
func (a *AppState) Address() string {
	return a.Username + "#" + a.Discriminator
}
