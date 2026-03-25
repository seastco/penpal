package server

import (
	"context"
	"crypto/ed25519"
	"fmt"
	"log"
	"sync"
	"time"

	pencrypto "github.com/seastco/penpal/internal/crypto"
	"github.com/seastco/penpal/internal/models"
)

// SystemUserID is an alias for models.SystemUserID, used throughout the server package.
var SystemUserID = models.SystemUserID

var (
	systemOnce    sync.Once
	systemPub     ed25519.PublicKey
	systemPriv    ed25519.PrivateKey
	systemReady   bool
	systemInitErr error
)

// InitSystemKeypair derives and caches the system user's ed25519 keypair from
// a BIP39 mnemonic (read from the PENPAL_SYSTEM_MNEMONIC env var at startup).
func InitSystemKeypair(mnemonic string) error {
	systemOnce.Do(func() {
		if mnemonic == "" {
			systemInitErr = fmt.Errorf("empty mnemonic")
			return
		}
		pub, priv, err := pencrypto.KeypairFromMnemonic(mnemonic)
		if err != nil {
			systemInitErr = fmt.Errorf("deriving system keypair: %w", err)
			return
		}
		systemPub = pub
		systemPriv = priv
		systemReady = true
	})
	return systemInitErr
}

// SystemKeypairReady returns true if the system keypair was successfully initialized.
func SystemKeypairReady() bool {
	return systemReady
}

const welcomeLetterBody = `Welcome to Penpal!

You've joined a network of penpals where every letter travels in real
time across the United States. No instant messaging here — your words
take the scenic route.

HOW SHIPPING WORKS

When you send a letter, it travels from your city to your friend's
city along a real route, hopping between relay stations. You can track
its journey as it moves. There are three shipping tiers:

  Standard     ~50 mph by ground truck     1 stamp
  Priority     ~100 mph ground + air       2 stamps
  Express      ~400 mph by air             3 stamps

A letter from New York to Los Angeles takes about 5 days by Standard,
2 days by Priority, or half a day by Express.

YOUR STAMPS

You started with 8 stamps — 3 random collectible stamps and 5 from
your home state. Every week you'll receive 2 more stamps just for
logging in.

When you send a letter, the stamps you use are consumed. When someone
sends you a letter, their stamps travel with it and transfer to you on
delivery. Collect them all!

GETTING STARTED

  1. Add a friend as a contact using their address (e.g. alice#1234)
  2. Compose a letter, pick a shipping tier, and attach your stamps
  3. Watch it travel across the country in real time

SETTINGS

Visit Settings from the home screen to:
  - Change your home city (this affects where letters depart from)
  - Set a PIN to lock your account
  - Switch between color themes

Happy writing!
— Steve from Penpal`

// Green Bay, WI coordinates
const (
	systemHomeLat = 44.5133
	systemHomeLng = -88.0133
)

// sendWelcomeLetter creates and inserts an already-delivered welcome letter
// from penpal#0000 to the newly registered user.
func (s *Server) sendWelcomeLetter(ctx context.Context, user *models.User) error {
	if !SystemKeypairReady() {
		return fmt.Errorf("system keypair not initialized")
	}

	encrypted, err := pencrypto.Encrypt([]byte(welcomeLetterBody), systemPriv, user.PublicKey)
	if err != nil {
		return fmt.Errorf("encrypting welcome letter: %w", err)
	}

	// Compute route from Green Bay, WI to user's home city
	gbIdx := s.graph.NearestCity(systemHomeLat, systemHomeLng)
	userIdx := s.graph.NearestCity(user.HomeLat, user.HomeLng)

	// Use a departure time 1 hour in the past so all hops appear completed
	backdate := time.Now().Add(-1 * time.Hour)
	route, _, err := s.graph.Route(gbIdx, userIdx, models.TierExpress, backdate)
	if err != nil {
		return fmt.Errorf("computing welcome route: %w", err)
	}

	// Backdate all ETAs to ensure the route shows as fully completed
	now := time.Now()
	for i := range route {
		route[i].ETA = now.Add(-time.Duration(len(route)-i) * time.Minute)
	}

	releaseAt := now.Add(-1 * time.Minute)
	msg := &models.Message{
		SenderID:      SystemUserID,
		RecipientID:   user.ID,
		EncryptedBody: encrypted,
		ShippingTier:  models.TierExpress,
		Route:         route,
		ReleaseAt:     releaseAt,
	}

	if err := s.db.CreateWelcomeMessage(ctx, msg); err != nil {
		return fmt.Errorf("inserting welcome message: %w", err)
	}

	log.Printf("sent welcome letter to %s#%s", user.Username, user.Discriminator)
	return nil
}
