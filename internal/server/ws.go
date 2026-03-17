package server

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"log"
	"math"
	mathrand "math/rand"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/stove/penpal/internal/models"
	"github.com/stove/penpal/internal/protocol"
	"github.com/stove/penpal/internal/routing"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Client represents a connected WebSocket client.
type Client struct {
	conn         *websocket.Conn
	userID       uuid.UUID
	server       *Server
	ip           string
	sendCh       chan protocol.Envelope
	pendingNonce []byte       // auth challenge nonce
	pendingUser  *models.User // user being authenticated
}

// commonStampPool is the set of common stamps awarded on registration and weekly.
var commonStampPool = []string{
	"common:flag", "common:heart", "common:star", "common:quill",
	"common:blossom", "common:sunflower", "common:butterfly", "common:wave",
	"common:moon", "common:bird", "common:rainbow", "common:clover",
}

// knownCountryCodes maps ISO 3166-1 alpha-2 codes for supported international countries.
var knownCountryCodes = map[string]bool{
	"ES": true, // Spain
}

// homeStampType returns the stamp type for a user's home city, e.g. "state:ma" or "country:es".
// Returns empty string if homeCity format is invalid.
func homeStampType(homeCity string) string {
	parts := strings.SplitN(homeCity, ", ", 2)
	if len(parts) != 2 {
		return ""
	}
	code := strings.TrimSpace(parts[1])
	if knownCountryCodes[strings.ToUpper(code)] {
		return "country:" + strings.ToLower(code)
	}
	return "state:" + strings.ToLower(code)
}

func (c *Client) Send(msgType string, payload any) {
	select {
	case c.sendCh <- protocol.Envelope{Type: protocol.MessageType(msgType), Payload: payload}:
	default:
		log.Printf("WARNING: sendCh full for user %s, dropping %s", c.userID, msgType)
	}
}

func (s *Server) handleWebSocket(w http.ResponseWriter, r *http.Request) {
	// IP rate limiting: max concurrent connections per IP
	ip := remoteIP(r, s.trustProxy)
	if !s.limiter.AllowConn(ip) {
		http.Error(w, "too many connections", http.StatusTooManyRequests)
		return
	}
	defer s.limiter.ReleaseConn(ip)

	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		// InsecureSkipVerify allows CLI clients (no Origin header).
		// This is safe because every mutating operation requires ed25519
		// challenge-response authentication — a browser cannot sign the
		// challenge without access to the user's private key.
		InsecureSkipVerify: true,
	})
	if err != nil {
		log.Printf("websocket accept error: %v", err)
		return
	}
	defer conn.CloseNow()
	conn.SetReadLimit(1 << 20) // 1 MB max message size

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()

	client := &Client{
		conn:   conn,
		server: s,
		ip:     ip,
		sendCh: make(chan protocol.Envelope, 64),
	}

	// Ensure hub cleanup runs even if a handler panics
	defer func() {
		if client.userID != uuid.Nil {
			s.hub.Unregister(client)
		}
	}()

	// Start send loop
	go client.sendLoop(ctx)

	// Server-side keepalive: ping every 30s, cancel ctx if client is dead
	go client.keepAlive(ctx, cancel)

	// Read loop — no per-read timeout; keepAlive detects dead connections
	for {
		var env protocol.Envelope
		err := wsjson.Read(ctx, conn, &env)
		if err != nil {
			if websocket.CloseStatus(err) == websocket.StatusNormalClosure {
				break
			}
			if ctx.Err() == nil {
				log.Printf("ws read error: %v", err)
			}
			break
		}

		if err := client.safeHandleMessage(ctx, env); err != nil {
			log.Printf("handler error for %s: %v", env.Type, err)
			client.sendError(env.ReqID, err.Error())
		}
	}
}

func (c *Client) safeHandleMessage(ctx context.Context, env protocol.Envelope) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = fmt.Errorf("panic: %v", r)
		}
	}()
	return c.handleMessage(ctx, env)
}

func (c *Client) keepAlive(ctx context.Context, cancel context.CancelFunc) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			pingCtx, pingCancel := context.WithTimeout(ctx, 10*time.Second)
			err := c.conn.Ping(pingCtx)
			pingCancel()
			if err != nil {
				cancel()
				return
			}
		}
	}
}

func (c *Client) sendLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case env := <-c.sendCh:
			ctx2, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := wsjson.Write(ctx2, c.conn, env)
			cancel()
			if err != nil {
				return
			}
		}
	}
}

func (c *Client) sendResponse(reqID string, msgType protocol.MessageType, payload any) {
	env := protocol.Envelope{
		Type:    msgType,
		Payload: payload,
		ReqID:   reqID,
	}
	select {
	case c.sendCh <- env:
	case <-time.After(10 * time.Second):
		log.Printf("sendResponse timeout for user %s, type %s", c.userID, msgType)
	}
}

func (c *Client) sendError(reqID string, msg string) {
	env := protocol.Envelope{
		Type:  protocol.MsgError,
		Error: msg,
		ReqID: reqID,
	}
	select {
	case c.sendCh <- env:
	case <-time.After(10 * time.Second):
		log.Printf("sendError timeout for user %s", c.userID)
	}
}

func (c *Client) handleMessage(ctx context.Context, env protocol.Envelope) error {
	switch env.Type {
	case protocol.MsgRegister:
		return c.handleRegister(ctx, env)
	case protocol.MsgAuth:
		return c.handleAuth(ctx, env)
	case protocol.MsgAuthResponse:
		return c.handleAuthResponse(ctx, env)
	case protocol.MsgRecover:
		return c.handleRecover(ctx, env)
	case protocol.MsgSendLetter:
		return c.requireAuth(func() error { return c.handleSendLetter(ctx, env) })
	case protocol.MsgGetInbox:
		return c.requireAuth(func() error { return c.handleGetInbox(ctx, env) })
	case protocol.MsgGetSent:
		return c.requireAuth(func() error { return c.handleGetSent(ctx, env) })
	case protocol.MsgGetInTransit:
		return c.requireAuth(func() error { return c.handleGetInTransit(ctx, env) })
	case protocol.MsgGetTracking:
		return c.requireAuth(func() error { return c.handleGetTracking(ctx, env) })
	case protocol.MsgMarkRead:
		return c.requireAuth(func() error { return c.handleMarkRead(ctx, env) })
	case protocol.MsgAddContact:
		return c.requireAuth(func() error { return c.handleAddContact(ctx, env) })
	case protocol.MsgGetContacts:
		return c.requireAuth(func() error { return c.handleGetContacts(ctx, env) })
	case protocol.MsgDeleteContact:
		return c.requireAuth(func() error { return c.handleDeleteContact(ctx, env) })
	case protocol.MsgBlockUser:
		return c.requireAuth(func() error { return c.handleBlockUser(ctx, env) })
	case protocol.MsgGetStamps:
		return c.requireAuth(func() error { return c.handleGetStamps(ctx, env) })
	case protocol.MsgGetMessage:
		return c.requireAuth(func() error { return c.handleGetMessage(ctx, env) })
	case protocol.MsgGetPublicKey:
		return c.requireAuth(func() error { return c.handleGetPublicKey(ctx, env) })
	case protocol.MsgSearchCities:
		return c.handleSearchCities(ctx, env) // no auth needed
	case protocol.MsgGetShipping:
		return c.requireAuth(func() error { return c.handleGetShipping(ctx, env) })
	case protocol.MsgUpdateHomeCity:
		return c.requireAuth(func() error { return c.handleUpdateHomeCity(ctx, env) })
	default:
		return fmt.Errorf("unknown message type: %s", env.Type)
	}
}

func (c *Client) requireAuth(fn func() error) error {
	if c.userID == uuid.Nil {
		return fmt.Errorf("not authenticated")
	}
	return fn()
}

func (c *Client) handleRegister(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.RegisterRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid register request: %w", err)
	}

	req.Username = strings.ToLower(req.Username)
	if len(req.Username) < 1 || len(req.Username) > 32 {
		return fmt.Errorf("username must be 1-32 characters")
	}
	if len(req.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size")
	}
	if len(req.HomeCity) > 200 {
		return fmt.Errorf("home city too long")
	}
	if !validCoords(req.HomeLat, req.HomeLng) {
		return fmt.Errorf("invalid coordinates")
	}

	// IP rate limiting: max registrations per IP per hour
	if !c.server.limiter.AllowRegistration(c.ip) {
		return fmt.Errorf("too many registrations, try again later")
	}

	user, err := c.server.db.CreateUser(ctx, req.Username, req.PublicKey, req.HomeCity, req.HomeLat, req.HomeLng)
	if err != nil {
		return fmt.Errorf("registration failed: %w", err)
	}

	// Award 2 distinct random common stamps
	pick1 := mathrand.Intn(len(commonStampPool))
	pick2 := mathrand.Intn(len(commonStampPool) - 1)
	if pick2 >= pick1 {
		pick2++
	}
	c.server.db.CreateStamp(ctx, user.ID, commonStampPool[pick1], models.RarityCommon, models.EarnedRegistration)
	c.server.db.CreateStamp(ctx, user.ID, commonStampPool[pick2], models.RarityCommon, models.EarnedRegistration)

	// Award home state/country stamp
	if st := homeStampType(req.HomeCity); st != "" {
		c.server.db.CreateStamp(ctx, user.ID, st, models.RarityCommon, models.EarnedRegistration)
	}

	c.userID = user.ID
	c.server.hub.Register(c)

	c.sendResponse(env.ReqID, protocol.MsgRegisterOK, protocol.RegisterResponse{
		UserID:        user.ID,
		Discriminator: user.Discriminator,
	})
	return nil
}

func (c *Client) handleAuth(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.AuthRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid auth request: %w", err)
	}

	user, err := c.server.db.GetUserByAddress(ctx, req.Username, strings.TrimSpace(req.Discriminator))
	if err != nil {
		return fmt.Errorf("auth lookup failed: %w", err)
	}
	if user == nil {
		return fmt.Errorf("user not found: %s#%s", req.Username, req.Discriminator)
	}

	// Generate nonce challenge
	nonce := make([]byte, 32)
	if _, err := rand.Read(nonce); err != nil {
		return fmt.Errorf("generating nonce: %w", err)
	}

	c.pendingNonce = nonce
	c.pendingUser = user

	c.sendResponse(env.ReqID, protocol.MsgAuthChallenge, protocol.AuthChallengeResponse{
		Nonce: nonce,
	})
	return nil
}

func (c *Client) handleAuthResponse(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.AuthResponsePayload
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid auth response: %w", err)
	}

	if c.pendingNonce == nil || c.pendingUser == nil {
		return fmt.Errorf("no pending auth challenge")
	}

	if !ed25519.Verify(c.pendingUser.PublicKey, c.pendingNonce, req.Signature) {
		c.pendingNonce = nil
		c.pendingUser = nil
		return fmt.Errorf("signature verification failed")
	}

	c.userID = c.pendingUser.ID
	c.server.hub.Register(c)
	c.server.db.TouchUserActive(ctx, c.userID)
	go c.server.awardWeeklyStamp(context.Background(), c.userID, c.pendingUser.HomeCity)

	user := *c.pendingUser
	c.pendingNonce = nil
	c.pendingUser = nil

	c.sendResponse(env.ReqID, protocol.MsgAuthOK, protocol.AuthOKResponse{
		User: user,
	})
	return nil
}

func (c *Client) handleRecover(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.RecoverRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid recover request: %w", err)
	}

	if len(req.PublicKey) != ed25519.PublicKeySize {
		return fmt.Errorf("invalid public key size")
	}

	user, err := c.server.db.GetUserByPublicKey(ctx, req.PublicKey)
	if err != nil {
		return fmt.Errorf("recovery lookup failed: %w", err)
	}
	if user == nil {
		return fmt.Errorf("no account found for this recovery phrase")
	}

	c.userID = user.ID
	c.server.hub.Register(c)
	c.server.db.TouchUserActive(ctx, c.userID)

	c.sendResponse(env.ReqID, protocol.MsgRecoverOK, protocol.RecoverResponse{
		User: *user,
	})
	return nil
}

func (c *Client) handleSendLetter(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.SendLetterRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid send request: %w", err)
	}

	if len(req.EncryptedBody) > 64*1024 {
		return fmt.Errorf("message too large")
	}
	if len(req.StampIDs) == 0 {
		return fmt.Errorf("at least one stamp is required")
	}
	if len(req.StampIDs) > 5 {
		return fmt.Errorf("too many stamps attached (max 5)")
	}

	// Validate sender has recipient as contact
	isContact, err := c.server.db.IsContact(ctx, c.userID, req.RecipientID)
	if err != nil {
		return err
	}
	if !isContact {
		// Allow sending if we've previously received a letter from this person
		hasReceived, err := c.server.db.HasReceivedFrom(ctx, c.userID, req.RecipientID)
		if err != nil {
			return err
		}
		if !hasReceived {
			return fmt.Errorf("recipient not in your contacts")
		}
	}

	// Check if blocked
	isBlocked, err := c.server.db.IsBlocked(ctx, req.RecipientID, c.userID)
	if err != nil {
		return err
	}
	if isBlocked {
		return fmt.Errorf("cannot send to this user")
	}

	// Rate limit
	ok, err := c.server.db.CheckRateLimit(ctx, c.userID, req.RecipientID)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("rate limit exceeded")
	}

	// Look up sender and recipient for routing
	sender, err := c.server.db.GetUserByID(ctx, c.userID)
	if err != nil || sender == nil {
		return fmt.Errorf("sender lookup failed")
	}
	recipient, err := c.server.db.GetUserByID(ctx, req.RecipientID)
	if err != nil || recipient == nil {
		return fmt.Errorf("recipient lookup failed")
	}

	// Find city indices
	fromIdx := c.server.graph.NearestCity(sender.HomeLat, sender.HomeLng)
	toIdx := c.server.graph.NearestCity(recipient.HomeLat, recipient.HomeLng)

	tier := models.ShippingTier(req.ShippingTier)
	if !models.ValidTier(tier) {
		return fmt.Errorf("invalid shipping tier: %s", req.ShippingTier)
	}

	// Determine if this is an international route
	isIntl := c.server.graph.Cities[fromIdx].EffectiveCountry() != c.server.graph.Cities[toIdx].EffectiveCountry()

	now := time.Now()
	route, totalDist, err := c.server.graph.Route(fromIdx, toIdx, tier, now, isIntl)
	if err != nil {
		return fmt.Errorf("route computation failed: %w", err)
	}

	// Compute release time (last hop ETA)
	releaseAt := route[len(route)-1].ETA

	msg := &models.Message{
		SenderID:      c.userID,
		RecipientID:   req.RecipientID,
		EncryptedBody: req.EncryptedBody,
		ShippingTier:  tier,
		Route:         route,
		ReleaseAt:     releaseAt,
	}

	if err := c.server.db.CreateMessage(ctx, msg, req.StampIDs); err != nil {
		return fmt.Errorf("storing message: %w", err)
	}

	c.sendResponse(env.ReqID, protocol.MsgLetterSent, protocol.LetterSentResponse{
		MessageID: msg.ID,
		Route:     route,
		ReleaseAt: releaseAt,
		Distance:  totalDist,
	})
	return nil
}

// awardWeeklyStamp awards 1 random stamp if 7+ days since last weekly award.
// Pool: 12 common stamps + user's home state/country stamp.
func (s *Server) awardWeeklyStamp(ctx context.Context, userID uuid.UUID, homeCity string) {
	lastWeekly, err := s.db.GetLastWeeklyStampTime(ctx, userID)
	if err != nil {
		return
	}
	if time.Since(lastWeekly) < 7*24*time.Hour {
		return
	}
	pool := make([]string, len(commonStampPool))
	copy(pool, commonStampPool)
	if st := homeStampType(homeCity); st != "" {
		pool = append(pool, st)
	}
	pick := pool[mathrand.Intn(len(pool))]
	stamp, err := s.db.CreateStamp(ctx, userID, pick, models.RarityCommon, models.EarnedWeekly)
	if err != nil {
		return
	}
	s.hub.SendToUser(userID, "stamp_awarded", protocol.StampAwardedPush{Stamp: *stamp})
}

func (c *Client) handleGetInbox(ctx context.Context, env protocol.Envelope) error {
	var req protocol.GetInboxRequest
	if env.Payload != nil {
		data, _ := json.Marshal(env.Payload)
		json.Unmarshal(data, &req)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	// Fetch limit+1 to detect has_more
	rows, err := c.server.db.GetInboxWithSenders(ctx, c.userID, req.Before, limit+1)
	if err != nil {
		return err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	// Batch-fetch stamps for all messages (1 query instead of N)
	msgIDs := make([]uuid.UUID, len(rows))
	for i, r := range rows {
		msgIDs[i] = r.ID
	}
	stampsByMsg, _ := c.server.db.GetStampsForMessages(ctx, msgIDs)

	items := make([]protocol.InboxItem, len(rows))
	for i, r := range rows {
		var deliveredAt time.Time
		if r.DeliveredAt != nil {
			deliveredAt = *r.DeliveredAt
		}
		items[i] = protocol.InboxItem{
			MessageID:     r.ID,
			SenderName:    r.SenderName,
			SenderID:      r.SenderID,
			SenderPubKey:  r.SenderPubKey,
			EncryptedBody: r.EncryptedBody,
			SentAt:        r.SentAt,
			DeliveredAt:   deliveredAt,
			ReadAt:        r.ReadAt,
			Stamps:        stampsByMsg[r.ID],
		}
	}

	c.sendResponse(env.ReqID, protocol.MsgInbox, protocol.InboxResponse{Letters: items, HasMore: hasMore})
	return nil
}

func (c *Client) handleGetSent(ctx context.Context, env protocol.Envelope) error {
	var req protocol.GetSentRequest
	if env.Payload != nil {
		data, _ := json.Marshal(env.Payload)
		json.Unmarshal(data, &req)
	}

	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 100
	}

	// Fetch limit+1 to detect has_more
	rows, err := c.server.db.GetSentWithRecipients(ctx, c.userID, req.Before, limit+1)
	if err != nil {
		return err
	}

	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}

	items := make([]protocol.SentItem, len(rows))
	for i, r := range rows {
		items[i] = protocol.SentItem{
			MessageID:     r.ID,
			RecipientName: r.RecipientName,
			RecipientID:   r.RecipientID,
			SentAt:        r.SentAt,
			ShippingTier:  string(r.ShippingTier),
			Status:        string(r.Status),
			Route:         r.Route,
		}
	}

	c.sendResponse(env.ReqID, protocol.MsgSentList, protocol.SentResponse{Letters: items, HasMore: hasMore})
	return nil
}

func (c *Client) handleGetInTransit(ctx context.Context, env protocol.Envelope) error {
	incoming, err := c.server.db.GetInTransitWithUsers(ctx, c.userID)
	if err != nil {
		return err
	}
	outgoing, err := c.server.db.GetOutgoingInTransitWithUsers(ctx, c.userID)
	if err != nil {
		return err
	}

	var items []protocol.InTransitItem

	for _, r := range incoming {
		items = append(items, protocol.InTransitItem{
			MessageID:    r.ID,
			Direction:    "incoming",
			PeerName:     r.SenderName,
			PeerID:       r.SenderID,
			OriginCity:   r.OriginCity,
			DestCity:     r.DestCity,
			ShippingTier: string(r.ShippingTier),
			Route:        r.Route,
			ReleaseAt:    r.ReleaseAt,
			SenderName:   r.SenderName,
			SenderID:     r.SenderID,
		})
	}

	for _, r := range outgoing {
		items = append(items, protocol.InTransitItem{
			MessageID:    r.ID,
			Direction:    "outgoing",
			PeerName:     r.RecipientName,
			PeerID:       r.RecipientID,
			OriginCity:   r.OriginCity,
			DestCity:     r.DestCity,
			ShippingTier: string(r.ShippingTier),
			Route:        r.Route,
			ReleaseAt:    r.ReleaseAt,
			SenderName:   r.RecipientName,
			SenderID:     r.SenderID,
		})
	}

	c.sendResponse(env.ReqID, protocol.MsgInTransitList, protocol.InTransitResponse{Letters: items})
	return nil
}

func (c *Client) handleGetTracking(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.GetTrackingRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid tracking request: %w", err)
	}

	msg, err := c.server.db.GetMessage(ctx, req.MessageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}
	if msg.SenderID != c.userID && msg.RecipientID != c.userID {
		return fmt.Errorf("not your message")
	}

	distance := routing.TotalDistance(msg.Route)
	c.sendResponse(env.ReqID, protocol.MsgTracking, protocol.TrackingResponse{
		MessageID:    msg.ID,
		Route:        msg.Route,
		ShippingTier: string(msg.ShippingTier),
		Status:       string(msg.Status),
		Distance:     distance,
	})
	return nil
}

func (c *Client) handleMarkRead(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.MarkReadRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid mark read request: %w", err)
	}
	// Verify the message belongs to the authenticated user
	msg, err := c.server.db.GetMessage(ctx, req.MessageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}
	if msg.RecipientID != c.userID {
		return fmt.Errorf("not authorized to mark this message as read")
	}
	if err := c.server.db.MarkRead(ctx, req.MessageID); err != nil {
		return err
	}
	c.sendResponse(env.ReqID, protocol.MsgMarkRead, nil)
	return nil
}

func (c *Client) handleAddContact(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.AddContactRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid add contact request: %w", err)
	}

	contact, err := c.server.db.GetUserByAddress(ctx, req.Username, strings.TrimSpace(req.Discriminator))
	if err != nil {
		return err
	}
	if contact == nil {
		return fmt.Errorf("user not found: %s#%s", req.Username, req.Discriminator)
	}
	if contact.ID == c.userID {
		return fmt.Errorf("cannot add yourself as a contact")
	}

	if err := c.server.db.AddContact(ctx, c.userID, contact.ID); err != nil {
		return err
	}

	c.sendResponse(env.ReqID, protocol.MsgContactsList, protocol.ContactItem{
		UserID:        contact.ID,
		Username:      contact.Username,
		Discriminator: contact.Discriminator,
		HomeCity:      contact.HomeCity,
	})
	return nil
}

func (c *Client) handleGetContacts(ctx context.Context, env protocol.Envelope) error {
	contacts, err := c.server.db.GetContacts(ctx, c.userID)
	if err != nil {
		return err
	}

	items := make([]protocol.ContactItem, len(contacts))
	for i, u := range contacts {
		items[i] = protocol.ContactItem{
			UserID:        u.ID,
			Username:      u.Username,
			Discriminator: u.Discriminator,
			HomeCity:      u.HomeCity,
		}
	}

	c.sendResponse(env.ReqID, protocol.MsgContactsList, protocol.ContactsResponse{Contacts: items})
	return nil
}

func (c *Client) handleDeleteContact(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.DeleteContactRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid delete contact request: %w", err)
	}
	if err := c.server.db.DeleteContact(ctx, c.userID, req.ContactID); err != nil {
		return fmt.Errorf("deleting contact: %w", err)
	}
	c.sendResponse(env.ReqID, protocol.MsgDeleteContactOK, nil)
	return nil
}

func (c *Client) handleBlockUser(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.BlockUserRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid block request: %w", err)
	}
	return c.server.db.BlockUser(ctx, c.userID, req.UserID)
}

func (c *Client) handleGetStamps(ctx context.Context, env protocol.Envelope) error {
	stamps, err := c.server.db.GetStamps(ctx, c.userID)
	if err != nil {
		return err
	}
	c.sendResponse(env.ReqID, protocol.MsgStampsList, protocol.StampsResponse{Stamps: stamps})
	return nil
}

func (c *Client) handleGetMessage(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.GetMessageRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid get message request: %w", err)
	}

	msg, err := c.server.db.GetMessage(ctx, req.MessageID)
	if err != nil {
		return err
	}
	if msg == nil {
		return fmt.Errorf("message not found")
	}
	// Only sender or recipient can read the message
	if msg.SenderID != c.userID && msg.RecipientID != c.userID {
		return fmt.Errorf("not authorized to read this message")
	}

	c.sendResponse(env.ReqID, protocol.MsgMessage, protocol.GetMessageResponse{
		MessageID:     msg.ID,
		SenderID:      msg.SenderID,
		EncryptedBody: msg.EncryptedBody,
	})
	return nil
}

func (c *Client) handleGetPublicKey(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.GetPublicKeyRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid get public key request: %w", err)
	}

	user, err := c.server.db.GetUserByID(ctx, req.UserID)
	if err != nil {
		return err
	}
	if user == nil {
		return fmt.Errorf("user not found")
	}

	c.sendResponse(env.ReqID, protocol.MsgPublicKey, protocol.PublicKeyResponse{PublicKey: user.PublicKey})
	return nil
}

func (c *Client) handleSearchCities(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.SearchCitiesRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid city search request: %w", err)
	}
	if len(req.Query) > 100 {
		return fmt.Errorf("search query too long")
	}
	limit := req.Limit
	if limit <= 0 || limit > 10 {
		limit = 5
	}

	cities := c.server.graph.SearchCities(req.Query, limit)
	results := make([]protocol.CityResult, len(cities))
	for i, c := range cities {
		results[i] = protocol.CityResult{
			Name:    c.Name,
			State:   c.State,
			Country: c.EffectiveCountry(),
			Lat:     c.Lat,
			Lng:     c.Lng,
		}
	}

	c.sendResponse(env.ReqID, protocol.MsgCityResults, protocol.CityResultsResponse{Cities: results})
	return nil
}

func (c *Client) handleGetShipping(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.GetShippingRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid shipping request: %w", err)
	}

	sender, err := c.server.db.GetUserByID(ctx, c.userID)
	if err != nil || sender == nil {
		return fmt.Errorf("sender lookup failed")
	}
	recipient, err := c.server.db.GetUserByID(ctx, req.RecipientID)
	if err != nil || recipient == nil {
		return fmt.Errorf("recipient lookup failed")
	}

	fromIdx := c.server.graph.NearestCity(sender.HomeLat, sender.HomeLng)
	toIdx := c.server.graph.NearestCity(recipient.HomeLat, recipient.HomeLng)

	// Determine if this is an international route
	isIntl := c.server.graph.Cities[fromIdx].EffectiveCountry() != c.server.graph.Cities[toIdx].EffectiveCountry()

	// Compute path once — Dijkstra result is tier-independent.
	path, dist, err := c.server.graph.Path(fromIdx, toIdx)
	if err != nil {
		return fmt.Errorf("route computation failed: %w", err)
	}

	var options []protocol.ShippingOption
	for _, tier := range models.AllTiers() {
		transitDays := routing.TransitDays(dist, tier, isIntl)
		options = append(options, protocol.ShippingOption{
			Tier:     string(tier),
			Days:     transitDays,
			Distance: dist,
			Hops:     len(path),
		})
	}

	c.sendResponse(env.ReqID, protocol.MsgShippingInfo, protocol.ShippingInfoResponse{
		FromCity: sender.HomeCity,
		ToCity:   recipient.HomeCity,
		Options:  options,
	})
	return nil
}

func (c *Client) handleUpdateHomeCity(ctx context.Context, env protocol.Envelope) error {
	data, _ := json.Marshal(env.Payload)
	var req protocol.UpdateHomeCityRequest
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid update home city request: %w", err)
	}

	city := strings.TrimSpace(req.City)
	if city == "" {
		return fmt.Errorf("city is required")
	}
	if !validCoords(req.Lat, req.Lng) {
		return fmt.Errorf("invalid coordinates")
	}

	// Validate that city is in the graph by finding nearest match
	idx := c.server.graph.NearestCity(req.Lat, req.Lng)
	graphCity := c.server.graph.Cities[idx]
	// Normalize to the graph city name
	city = graphCity.FullName()

	if err := c.server.db.UpdateHomeCity(ctx, c.userID, city, graphCity.Lat, graphCity.Lng); err != nil {
		return fmt.Errorf("updating home city: %w", err)
	}

	c.sendResponse(env.ReqID, protocol.MsgHomeCityUpdated, nil)
	return nil
}

// validCoords returns true if lat/lng are finite and within Earth bounds.
func validCoords(lat, lng float64) bool {
	return !math.IsNaN(lat) && !math.IsNaN(lng) &&
		!math.IsInf(lat, 0) && !math.IsInf(lng, 0) &&
		lat >= -90 && lat <= 90 &&
		lng >= -180 && lng <= 180
}

// remoteIP extracts the client IP from the request.
func remoteIP(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if fwd := r.Header.Get("X-Forwarded-For"); fwd != "" {
			// First entry is the original client IP
			if i := strings.IndexByte(fwd, ','); i != -1 {
				return strings.TrimSpace(fwd[:i])
			}
			return strings.TrimSpace(fwd)
		}
	}
	// Strip port from RemoteAddr
	addr := r.RemoteAddr
	if i := strings.LastIndex(addr, ":"); i != -1 {
		return addr[:i]
	}
	return addr
}
