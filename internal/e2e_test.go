package e2e

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	pencrypto "github.com/stove/penpal/internal/crypto"
	"github.com/stove/penpal/internal/protocol"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

const serverURL = "ws://localhost:8282"

// wsClient is a minimal WebSocket client for testing.
type wsClient struct {
	conn *websocket.Conn
	seq  int
}

func dial(t *testing.T) *wsClient {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, serverURL+"/v1/ws", nil)
	if err != nil {
		t.Fatalf("connecting to server: %v", err)
	}
	conn.SetReadLimit(1 << 20)
	return &wsClient{conn: conn}
}

func (c *wsClient) close() {
	c.conn.Close(websocket.StatusNormalClosure, "bye")
}

func (c *wsClient) send(t *testing.T, msgType protocol.MessageType, payload any) protocol.Envelope {
	t.Helper()
	c.seq++
	reqID := fmt.Sprintf("test-%d", c.seq)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	env := protocol.Envelope{
		Type:    msgType,
		Payload: payload,
		ReqID:   reqID,
	}
	if err := wsjson.Write(ctx, c.conn, env); err != nil {
		t.Fatalf("sending %s: %v", msgType, err)
	}

	// Read until we get our response
	for {
		var resp protocol.Envelope
		if err := wsjson.Read(ctx, c.conn, &resp); err != nil {
			t.Fatalf("reading response for %s: %v", msgType, err)
		}
		if resp.ReqID == reqID {
			if resp.Error != "" {
				t.Fatalf("server error for %s: %s", msgType, resp.Error)
			}
			return resp
		}
		// Push notification, skip
	}
}

func unmarshal[T any](t *testing.T, env protocol.Envelope) T {
	t.Helper()
	data, _ := json.Marshal(env.Payload)
	var result T
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatalf("unmarshaling %s response: %v", env.Type, err)
	}
	return result
}

func TestEndToEnd(t *testing.T) {
	// ===== Step 1: Register User A (Steven in Boston) =====
	t.Log("=== Registering User A (steven) ===")
	mnemonicA, _ := pencrypto.GenerateMnemonic()
	pubA, privA, _ := pencrypto.KeypairFromMnemonic(mnemonicA)

	clientA := dial(t)
	defer clientA.close()

	respA := clientA.send(t, protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "steven",
		PublicKey: pubA,
		HomeCity:  "Boston, MA",
		HomeLat:   42.3601,
		HomeLng:   -71.0589,
	})
	regA := unmarshal[protocol.RegisterResponse](t, respA)
	t.Logf("Steven registered as steven#%s (ID: %s)", regA.Discriminator, regA.UserID)

	// ===== Step 2: Register User B (Jake in Denver) =====
	t.Log("=== Registering User B (jake) ===")
	mnemonicB, _ := pencrypto.GenerateMnemonic()
	pubB, privB, _ := pencrypto.KeypairFromMnemonic(mnemonicB)

	clientB := dial(t)
	defer clientB.close()

	respB := clientB.send(t, protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "jake",
		PublicKey: pubB,
		HomeCity:  "Denver, CO",
		HomeLat:   39.7392,
		HomeLng:   -104.9903,
	})
	regB := unmarshal[protocol.RegisterResponse](t, respB)
	t.Logf("Jake registered as jake#%s (ID: %s)", regB.Discriminator, regB.UserID)

	// ===== Step 3: Steven adds Jake as a contact =====
	t.Log("=== Steven adds Jake as contact ===")
	clientA.send(t, protocol.MsgAddContact, protocol.AddContactRequest{
		Username:      "jake",
		Discriminator: regB.Discriminator,
	})
	t.Log("Contact added")

	// ===== Step 4: Verify contacts list =====
	contactsResp := clientA.send(t, protocol.MsgGetContacts, nil)
	contacts := unmarshal[protocol.ContactsResponse](t, contactsResp)
	if len(contacts.Contacts) != 1 {
		t.Fatalf("expected 1 contact, got %d", len(contacts.Contacts))
	}
	if contacts.Contacts[0].Username != "jake" {
		t.Fatalf("expected jake, got %s", contacts.Contacts[0].Username)
	}
	t.Logf("Contacts: %s (%s)", contacts.Contacts[0].Username, contacts.Contacts[0].HomeCity)

	// ===== Step 5: Get shipping info =====
	t.Log("=== Checking shipping options ===")
	shippingResp := clientA.send(t, protocol.MsgGetShipping, protocol.GetShippingRequest{
		RecipientID: regB.UserID,
	})
	shipping := unmarshal[protocol.ShippingInfoResponse](t, shippingResp)
	t.Logf("Route: %s -> %s", shipping.FromCity, shipping.ToCity)
	for _, opt := range shipping.Options {
		t.Logf("  %s: %.1f days, %.0f mi, %d hops", opt.Tier, opt.Days, opt.Distance, opt.Hops)
	}

	// ===== Step 6: Get Steven's stamps (needed to attach one to the letter) =====
	t.Log("=== Steven's stamps ===")
	stampsResp := clientA.send(t, protocol.MsgGetStamps, nil)
	stamps := unmarshal[protocol.StampsResponse](t, stampsResp)
	t.Logf("Steven has %d stamps", len(stamps.Stamps))
	if len(stamps.Stamps) != 3 {
		t.Fatalf("expected 3 registration stamps (2 common + 1 state), got %d", len(stamps.Stamps))
	}
	for _, s := range stamps.Stamps {
		if s.EarnedVia != "registration" {
			t.Fatalf("expected earned_via=registration, got %s for %s", s.EarnedVia, s.StampType)
		}
		isCommon := strings.HasPrefix(s.StampType, "common:")
		isState := strings.HasPrefix(s.StampType, "state:")
		isCountry := strings.HasPrefix(s.StampType, "country:")
		if !isCommon && !isState && !isCountry {
			t.Fatalf("unexpected stamp type: %s", s.StampType)
		}
	}
	for _, s := range stamps.Stamps {
		t.Logf("  %s (%s) — earned via %s", s.StampType, s.Rarity, s.EarnedVia)
	}

	// ===== Step 7: Get Jake's public key and encrypt a letter =====
	t.Log("=== Encrypting letter ===")
	pkResp := clientA.send(t, protocol.MsgGetPublicKey, protocol.GetPublicKeyRequest{
		UserID: regB.UserID,
	})
	pkResult := unmarshal[protocol.PublicKeyResponse](t, pkResp)

	letterBody := "Hey Jake,\n\nHaven't heard from you in a while. How's Denver?\n\n-- steven"
	encrypted, err := pencrypto.Encrypt([]byte(letterBody), privA, ed25519.PublicKey(pkResult.PublicKey))
	if err != nil {
		t.Fatalf("encrypting letter: %v", err)
	}
	t.Logf("Encrypted letter: %d bytes plaintext -> %d bytes ciphertext", len(letterBody), len(encrypted))

	// ===== Step 8: Send the letter (with a stamp attached) =====
	t.Log("=== Sending letter (priority shipping) ===")
	sendResp := clientA.send(t, protocol.MsgSendLetter, protocol.SendLetterRequest{
		RecipientID:   regB.UserID,
		EncryptedBody: encrypted,
		ShippingTier:  "priority",
		StampIDs:      []uuid.UUID{stamps.Stamps[0].ID},
	})
	sentResult := unmarshal[protocol.LetterSentResponse](t, sendResp)
	t.Logf("Letter sent! ID: %s", sentResult.MessageID)
	t.Logf("Distance: %.0f mi, %d hops, arrives ~%s",
		sentResult.Distance, len(sentResult.Route),
		sentResult.ReleaseAt.Format("Jan 2 15:04"))

	// Print relay log
	t.Log("=== Relay Log ===")
	for _, hop := range sentResult.Route {
		t.Logf("  %s  %s  %s", hop.ETA.Format("01/02 15:04"), hop.Relay, hop.City)
	}

	// ===== Step 8: Jake checks in-transit =====
	t.Log("=== Jake checks in-transit ===")
	transitResp := clientB.send(t, protocol.MsgGetInTransit, nil)
	transit := unmarshal[protocol.InTransitResponse](t, transitResp)
	if len(transit.Letters) != 1 {
		t.Fatalf("expected 1 in-transit letter, got %d", len(transit.Letters))
	}
	t.Logf("Jake sees incoming letter from %s via %s", transit.Letters[0].SenderName, transit.Letters[0].ShippingTier)

	// ===== Step 9: Steven checks sent =====
	t.Log("=== Steven checks sent ===")
	sentListResp := clientA.send(t, protocol.MsgGetSent, nil)
	sentList := unmarshal[protocol.SentResponse](t, sentListResp)
	if len(sentList.Letters) != 1 {
		t.Fatalf("expected 1 sent letter, got %d", len(sentList.Letters))
	}
	if sentList.Letters[0].Status != "in_transit" {
		t.Fatalf("expected in_transit, got %s", sentList.Letters[0].Status)
	}
	t.Logf("Steven's sent: to %s, status %s", sentList.Letters[0].RecipientName, sentList.Letters[0].Status)

	// ===== Step 10: Steven checks tracking =====
	t.Log("=== Tracking ===")
	trackResp := clientA.send(t, protocol.MsgGetTracking, protocol.GetTrackingRequest{
		MessageID: sentResult.MessageID,
	})
	tracking := unmarshal[protocol.TrackingResponse](t, trackResp)
	t.Logf("Tracking: %s, %.0f mi, status %s", tracking.ShippingTier, tracking.Distance, tracking.Status)

	// ===== Step 11: Test auth flow (disconnect and reconnect) =====
	t.Log("=== Testing auth reconnect ===")
	clientA2 := dial(t)
	defer clientA2.close()

	// Request auth challenge
	clientA2.seq = 100
	authResp := clientA2.send(t, protocol.MsgAuth, protocol.AuthRequest{
		Username:      "steven",
		Discriminator: regA.Discriminator,
	})
	challenge := unmarshal[protocol.AuthChallengeResponse](t, authResp)
	t.Logf("Got auth challenge: %d byte nonce", len(challenge.Nonce))

	// Sign nonce
	signature := ed25519.Sign(privA, challenge.Nonce)

	authOKResp := clientA2.send(t, protocol.MsgAuthResponse, protocol.AuthResponsePayload{
		Signature: signature,
	})
	authOK := unmarshal[protocol.AuthOKResponse](t, authOKResp)
	t.Logf("Re-authenticated as %s#%s", authOK.User.Username, authOK.User.Discriminator)

	// ===== Step 12: Test account recovery via server =====
	t.Log("=== Testing account recovery ===")

	// Open a fresh connection (simulating a new device)
	clientA3 := dial(t)
	defer clientA3.close()
	clientA3.seq = 200

	// Derive keypair from the same mnemonic
	pubRecovered, _, _ := pencrypto.KeypairFromMnemonic(mnemonicA)
	if !pubA.Equal(pubRecovered) {
		t.Fatal("recovered public key doesn't match original")
	}

	// Send recovery request to server
	recoverResp := clientA3.send(t, protocol.MsgRecover, protocol.RecoverRequest{
		PublicKey: pubRecovered,
	})
	recoverResult := unmarshal[protocol.RecoverResponse](t, recoverResp)

	if recoverResult.User.ID != regA.UserID {
		t.Fatalf("recovered user ID mismatch: got %s, want %s", recoverResult.User.ID, regA.UserID)
	}
	if recoverResult.User.Username != "steven" {
		t.Fatalf("recovered username mismatch: got %s, want steven", recoverResult.User.Username)
	}
	if recoverResult.User.Discriminator != regA.Discriminator {
		t.Fatalf("recovered discriminator mismatch: got %s, want %s",
			recoverResult.User.Discriminator, regA.Discriminator)
	}
	if recoverResult.User.HomeCity != "Boston, MA" {
		t.Fatalf("recovered home city mismatch: got %s, want Boston, MA", recoverResult.User.HomeCity)
	}
	t.Logf("Recovery: restored %s#%s (ID: %s) from mnemonic",
		recoverResult.User.Username, recoverResult.User.Discriminator, recoverResult.User.ID)

	// Verify the recovered session is authenticated by making an authenticated request
	stampsResp2 := clientA3.send(t, protocol.MsgGetStamps, nil)
	stamps2 := unmarshal[protocol.StampsResponse](t, stampsResp2)
	if len(stamps2.Stamps) < 2 {
		t.Fatalf("expected at least 2 stamps after recovery, got %d", len(stamps2.Stamps))
	}
	t.Logf("Recovery session authenticated: can see %d stamps", len(stamps2.Stamps))

	// Test recovery with unknown mnemonic (should fail)
	t.Log("=== Testing recovery with unknown mnemonic ===")
	clientA4 := dial(t)
	defer clientA4.close()
	clientA4.seq = 300
	unknownMnemonic, _ := pencrypto.GenerateMnemonic()
	unknownPub, _, _ := pencrypto.KeypairFromMnemonic(unknownMnemonic)

	func() {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		env := protocol.Envelope{
			Type:    protocol.MsgRecover,
			Payload: protocol.RecoverRequest{PublicKey: unknownPub},
			ReqID:   "test-fail-recover",
		}
		if err := wsjson.Write(ctx, clientA4.conn, env); err != nil {
			t.Fatalf("sending recover: %v", err)
		}
		var resp protocol.Envelope
		if err := wsjson.Read(ctx, clientA4.conn, &resp); err != nil {
			t.Fatalf("reading recover response: %v", err)
		}
		if resp.Error == "" {
			t.Fatal("expected error for unknown recovery key, got success")
		}
		t.Logf("Unknown mnemonic correctly rejected: %s", resp.Error)
	}()

	// ===== Step 13: Test decryption (verify Jake can decrypt) =====
	t.Log("=== Testing decryption ===")
	decrypted, err := pencrypto.Decrypt(encrypted, privB, pubA)
	if err != nil {
		t.Fatalf("Jake failed to decrypt: %v", err)
	}
	if string(decrypted) != letterBody {
		t.Fatalf("decrypted text mismatch:\n  got: %q\n  want: %q", string(decrypted), letterBody)
	}
	t.Logf("Jake decrypted: %q", string(decrypted)[:50]+"...")

	// ===== Step 14: City search =====
	t.Log("=== City search ===")
	cityResp := clientA.send(t, protocol.MsgSearchCities, protocol.SearchCitiesRequest{
		Query: "den",
		Limit: 3,
	})
	cities := unmarshal[protocol.CityResultsResponse](t, cityResp)
	for _, c := range cities.Cities {
		t.Logf("  %s, %s (%.4f, %.4f)", c.Name, c.State, c.Lat, c.Lng)
	}

	t.Log("=== END-TO-END TEST COMPLETE ===")
}
