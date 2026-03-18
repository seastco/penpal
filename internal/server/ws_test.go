package server

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/models"
	"github.com/seastco/penpal/internal/protocol"
	"github.com/seastco/penpal/internal/routing"
)

// testServer creates a Server with a mock store and a small test graph.
func testServer(t *testing.T, store *mockStore) *Server {
	t.Helper()

	cities := []routing.City{
		{Name: "Boston", State: "MA", Lat: 42.36, Lng: -71.06},
		{Name: "New York", State: "NY", Lat: 40.71, Lng: -74.01},
		{Name: "Denver", State: "CO", Lat: 39.74, Lng: -104.99},
	}
	graph := routing.NewGraph(cities)

	return &Server{
		db:      store,
		graph:   graph,
		hub:     NewHub(),
		limiter: NewIPRateLimiter(),
	}
}

// testClient creates a Client attached to the given server with a sendCh for capturing responses.
func testClient(s *Server) *Client {
	return &Client{
		server: s,
		sendCh: make(chan protocol.Envelope, 64),
		ip:     "127.0.0.1",
	}
}

// readResponse drains the first envelope from sendCh.
func readResponse(t *testing.T, c *Client) protocol.Envelope {
	t.Helper()
	select {
	case env := <-c.sendCh:
		return env
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for response")
		return protocol.Envelope{}
	}
}

// makeEnvelope creates an Envelope with the given type and payload.
func makeEnvelope(msgType protocol.MessageType, payload any) protocol.Envelope {
	return protocol.Envelope{
		Type:    msgType,
		Payload: payload,
		ReqID:   uuid.New().String(),
	}
}

func TestHandleRegister_Success(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	pub, _, _ := ed25519.GenerateKey(nil)

	env := makeEnvelope(protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "alice",
		PublicKey: pub,
		HomeCity:  "Boston, MA",
		HomeLat:   42.36,
		HomeLng:   -71.06,
	})

	if err := c.handleMessage(context.Background(), env); err != nil {
		t.Fatalf("handleRegister: %v", err)
	}

	resp := readResponse(t, c)
	if resp.Type != protocol.MsgRegisterOK {
		t.Fatalf("expected register_ok, got %s", resp.Type)
	}

	// Client should now be authenticated
	if c.userID == uuid.Nil {
		t.Fatal("client should have userID set after register")
	}

	// Stamps should have been awarded (3 common + 5 state = 8)
	stamps, _ := store.GetStamps(context.Background(), c.userID)
	if len(stamps) != 8 {
		t.Fatalf("expected 8 stamps (3 common + 5 state), got %d", len(stamps))
	}
}

func TestHandleRegister_InvalidUsername_Empty(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	pub, _, _ := ed25519.GenerateKey(nil)
	env := makeEnvelope(protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "",
		PublicKey: pub,
		HomeCity:  "Boston, MA",
		HomeLat:   42.36,
		HomeLng:   -71.06,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for empty username")
	}
}

func TestHandleRegister_InvalidPublicKey(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	env := makeEnvelope(protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "alice",
		PublicKey: []byte("too-short"),
		HomeCity:  "Boston, MA",
		HomeLat:   42.36,
		HomeLng:   -71.06,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for invalid public key")
	}
}

func TestHandleRegister_InvalidCoords(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	pub, _, _ := ed25519.GenerateKey(nil)
	env := makeEnvelope(protocol.MsgRegister, protocol.RegisterRequest{
		Username:  "alice",
		PublicKey: pub,
		HomeCity:  "Boston, MA",
		HomeLat:   999,
		HomeLng:   -71.06,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for invalid coordinates")
	}
}

func TestHandleAuth_ChallengeResponse(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	// Create a user with known keys
	pub, priv, _ := ed25519.GenerateKey(nil)
	user := &models.User{
		ID:            uuid.New(),
		Username:      "alice",
		Discriminator: "1234",
		PublicKey:     pub,
		HomeCity:      "Boston, MA",
		HomeLat:       42.36,
		HomeLng:       -71.06,
		LastActive:    time.Now(),
		CreatedAt:     time.Now(),
	}
	store.addUser(user)

	// Step 1: Send auth request
	authEnv := makeEnvelope(protocol.MsgAuth, protocol.AuthRequest{
		Username:      "alice",
		Discriminator: "1234",
	})
	if err := c.handleMessage(context.Background(), authEnv); err != nil {
		t.Fatalf("handleAuth: %v", err)
	}

	// Should get a challenge back
	challenge := readResponse(t, c)
	if challenge.Type != protocol.MsgAuthChallenge {
		t.Fatalf("expected auth_challenge, got %s", challenge.Type)
	}

	// Extract nonce from challenge
	data, _ := json.Marshal(challenge.Payload)
	var challengeResp protocol.AuthChallengeResponse
	json.Unmarshal(data, &challengeResp)

	if len(challengeResp.Nonce) != 32 {
		t.Fatalf("expected 32-byte nonce, got %d bytes", len(challengeResp.Nonce))
	}

	// Step 2: Sign the nonce and respond
	sig := ed25519.Sign(priv, challengeResp.Nonce)
	authRespEnv := makeEnvelope(protocol.MsgAuthResponse, protocol.AuthResponsePayload{
		Signature: sig,
	})
	if err := c.handleMessage(context.Background(), authRespEnv); err != nil {
		t.Fatalf("handleAuthResponse: %v", err)
	}

	// Should get auth_ok back
	authOK := readResponse(t, c)
	if authOK.Type != protocol.MsgAuthOK {
		t.Fatalf("expected auth_ok, got %s", authOK.Type)
	}

	// Client should be authenticated
	if c.userID != user.ID {
		t.Fatalf("expected userID %s, got %s", user.ID, c.userID)
	}
}

func TestHandleAuth_BadSignature(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	pub, _, _ := ed25519.GenerateKey(nil)
	user := &models.User{
		ID:            uuid.New(),
		Username:      "alice",
		Discriminator: "1234",
		PublicKey:     pub,
		HomeCity:      "Boston, MA",
		HomeLat:       42.36,
		HomeLng:       -71.06,
		LastActive:    time.Now(),
		CreatedAt:     time.Now(),
	}
	store.addUser(user)

	// Send auth request
	authEnv := makeEnvelope(protocol.MsgAuth, protocol.AuthRequest{
		Username:      "alice",
		Discriminator: "1234",
	})
	c.handleMessage(context.Background(), authEnv)
	readResponse(t, c) // drain challenge

	// Send wrong signature
	authRespEnv := makeEnvelope(protocol.MsgAuthResponse, protocol.AuthResponsePayload{
		Signature: make([]byte, 64), // all zeros — invalid
	})
	err := c.handleMessage(context.Background(), authRespEnv)
	if err == nil {
		t.Fatal("expected error for bad signature")
	}

	// Client should NOT be authenticated
	if c.userID != uuid.Nil {
		t.Fatal("client should not be authenticated after bad signature")
	}
}

func TestHandleAuth_UserNotFound(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	authEnv := makeEnvelope(protocol.MsgAuth, protocol.AuthRequest{
		Username:      "nobody",
		Discriminator: "0000",
	})
	err := c.handleMessage(context.Background(), authEnv)
	if err == nil {
		t.Fatal("expected error for nonexistent user")
	}
}

func TestRequireAuth_Unauthenticated(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	// Try to get inbox without authenticating
	env := makeEnvelope(protocol.MsgGetInbox, nil)
	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for unauthenticated request")
	}
}

func TestHandleSendLetter_NotContact(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	// Create sender and recipient
	senderPub, _, _ := ed25519.GenerateKey(nil)
	sender := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		PublicKey: senderPub, HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	recipient := &models.User{
		ID: uuid.New(), Username: "bob", Discriminator: "0002",
		HomeCity: "Denver, CO", HomeLat: 39.74, HomeLng: -104.99,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(sender)
	store.addUser(recipient)

	// Authenticate as sender
	c.userID = sender.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgSendLetter, protocol.SendLetterRequest{
		RecipientID:   recipient.ID,
		EncryptedBody: []byte("hello"),
		ShippingTier:  "first_class",
		StampIDs:      []uuid.UUID{uuid.New()},
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for non-contact recipient")
	}
}

func TestHandleSendLetter_RateLimited(t *testing.T) {
	store := newMockStore()
	store.rateLimitOK = false // force rate limit rejection
	s := testServer(t, store)
	c := testClient(s)

	sender := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	recipient := &models.User{
		ID: uuid.New(), Username: "bob", Discriminator: "0002",
		HomeCity: "Denver, CO", HomeLat: 39.74, HomeLng: -104.99,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(sender)
	store.addUser(recipient)
	store.AddContact(context.Background(), sender.ID, recipient.ID)

	c.userID = sender.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgSendLetter, protocol.SendLetterRequest{
		RecipientID:   recipient.ID,
		EncryptedBody: []byte("hello"),
		ShippingTier:  "first_class",
		StampIDs:      []uuid.UUID{uuid.New()},
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for rate limited send")
	}
}

func TestHandleSendLetter_Blocked(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	sender := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	recipient := &models.User{
		ID: uuid.New(), Username: "bob", Discriminator: "0002",
		HomeCity: "Denver, CO", HomeLat: 39.74, HomeLng: -104.99,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(sender)
	store.addUser(recipient)
	store.AddContact(context.Background(), sender.ID, recipient.ID)
	store.BlockUser(context.Background(), recipient.ID, sender.ID) // bob blocks alice

	c.userID = sender.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgSendLetter, protocol.SendLetterRequest{
		RecipientID:   recipient.ID,
		EncryptedBody: []byte("hello"),
		ShippingTier:  "first_class",
		StampIDs:      []uuid.UUID{uuid.New()},
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for blocked sender")
	}
}

func TestHandleSendLetter_Success(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	sender := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	recipient := &models.User{
		ID: uuid.New(), Username: "bob", Discriminator: "0002",
		HomeCity: "Denver, CO", HomeLat: 39.74, HomeLng: -104.99,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(sender)
	store.addUser(recipient)
	store.AddContact(context.Background(), sender.ID, recipient.ID)

	c.userID = sender.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgSendLetter, protocol.SendLetterRequest{
		RecipientID:   recipient.ID,
		EncryptedBody: []byte("hello"),
		ShippingTier:  "first_class",
		StampIDs:      []uuid.UUID{uuid.New()},
	})

	if err := c.handleMessage(context.Background(), env); err != nil {
		t.Fatalf("handleSendLetter: %v", err)
	}

	resp := readResponse(t, c)
	if resp.Type != protocol.MsgLetterSent {
		t.Fatalf("expected letter_sent, got %s", resp.Type)
	}

	// Verify message was stored
	store.mu.Lock()
	msgCount := len(store.messages)
	store.mu.Unlock()
	if msgCount != 1 {
		t.Fatalf("expected 1 message in store, got %d", msgCount)
	}
}

func TestHandleGetContacts(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	user := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	contact := &models.User{
		ID: uuid.New(), Username: "bob", Discriminator: "0002",
		HomeCity: "Denver, CO", HomeLat: 39.74, HomeLng: -104.99,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(user)
	store.addUser(contact)
	store.AddContact(context.Background(), user.ID, contact.ID)

	c.userID = user.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgGetContacts, nil)
	if err := c.handleMessage(context.Background(), env); err != nil {
		t.Fatalf("handleGetContacts: %v", err)
	}

	resp := readResponse(t, c)
	if resp.Type != protocol.MsgContactsList {
		t.Fatalf("expected contacts_list, got %s", resp.Type)
	}
}

func TestHandleAddContact_Self(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	user := &models.User{
		ID: uuid.New(), Username: "alice", Discriminator: "0001",
		HomeCity: "Boston, MA", HomeLat: 42.36, HomeLng: -71.06,
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(user)

	c.userID = user.ID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgAddContact, protocol.AddContactRequest{
		UserID: user.ID,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error when adding self as contact")
	}
}

func TestHandleMarkRead(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	userID := uuid.New()
	user := &models.User{
		ID: userID, Username: "alice", Discriminator: "0001",
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(user)

	// Create a delivered message to this user
	now := time.Now()
	msg := &models.Message{
		ID:          uuid.New(),
		SenderID:    uuid.New(),
		RecipientID: userID,
		Status:      "delivered",
		DeliveredAt: &now,
	}
	store.mu.Lock()
	store.messages[msg.ID] = msg
	store.mu.Unlock()

	c.userID = userID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgMarkRead, protocol.MarkReadRequest{
		MessageID: msg.ID,
	})

	if err := c.handleMessage(context.Background(), env); err != nil {
		t.Fatalf("handleMarkRead: %v", err)
	}

	resp := readResponse(t, c)
	if resp.Type != protocol.MsgMarkRead {
		t.Fatalf("expected mark_read, got %s", resp.Type)
	}

	// Verify status changed
	if msg.Status != "read" {
		t.Fatalf("expected status 'read', got %q", msg.Status)
	}
}

func TestHandleMarkRead_NotYourMessage(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	userID := uuid.New()
	user := &models.User{
		ID: userID, Username: "alice", Discriminator: "0001",
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(user)

	// Message belongs to someone else
	now := time.Now()
	msg := &models.Message{
		ID:          uuid.New(),
		SenderID:    uuid.New(),
		RecipientID: uuid.New(), // not our user
		Status:      "delivered",
		DeliveredAt: &now,
	}
	store.mu.Lock()
	store.messages[msg.ID] = msg
	store.mu.Unlock()

	c.userID = userID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgMarkRead, protocol.MarkReadRequest{
		MessageID: msg.ID,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for marking someone else's message as read")
	}
}

func TestHandleSearchCities(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	// SearchCities doesn't require auth
	env := makeEnvelope(protocol.MsgSearchCities, protocol.SearchCitiesRequest{
		Query: "Bos",
		Limit: 5,
	})

	if err := c.handleMessage(context.Background(), env); err != nil {
		t.Fatalf("handleSearchCities: %v", err)
	}

	resp := readResponse(t, c)
	if resp.Type != protocol.MsgCityResults {
		t.Fatalf("expected city_results, got %s", resp.Type)
	}

	data, _ := json.Marshal(resp.Payload)
	var results protocol.CityResultsResponse
	json.Unmarshal(data, &results)

	if len(results.Cities) == 0 {
		t.Fatal("expected at least one city result for 'Bos'")
	}
	if results.Cities[0].Name != "Boston" {
		t.Fatalf("expected 'Boston', got %q", results.Cities[0].Name)
	}
}

func TestHandleGetTracking_NotYourMessage(t *testing.T) {
	store := newMockStore()
	s := testServer(t, store)
	c := testClient(s)

	userID := uuid.New()
	user := &models.User{
		ID: userID, Username: "alice", Discriminator: "0001",
		LastActive: time.Now(), CreatedAt: time.Now(),
	}
	store.addUser(user)

	msg := &models.Message{
		ID:          uuid.New(),
		SenderID:    uuid.New(),
		RecipientID: uuid.New(), // neither sender nor recipient
		Status:      "in_transit",
	}
	store.mu.Lock()
	store.messages[msg.ID] = msg
	store.mu.Unlock()

	c.userID = userID
	s.hub.Register(c)

	env := makeEnvelope(protocol.MsgGetTracking, protocol.GetTrackingRequest{
		MessageID: msg.ID,
	})

	err := c.handleMessage(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for tracking someone else's message")
	}
}
