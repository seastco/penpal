package client

import (
	"context"
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/google/uuid"
	"github.com/stove/penpal/internal/protocol"
	"nhooyr.io/websocket"
	"nhooyr.io/websocket/wsjson"
)

// Network manages the WebSocket connection to the relay server.
type Network struct {
	serverURL string
	conn      *websocket.Conn
	connected atomic.Bool

	// Request-response correlation
	mu       sync.Mutex
	pending  map[string]chan protocol.Envelope
	reqSeq   int64
	writeMu  sync.Mutex // protects concurrent WebSocket writes

	// Push notification callback
	onPush func(protocol.Envelope)
}

// NewNetwork creates a new network client.
func NewNetwork(serverURL string) *Network {
	return &Network{
		serverURL: serverURL,
		pending:   make(map[string]chan protocol.Envelope),
	}
}

// Connect establishes the WebSocket connection.
func (n *Network) Connect(ctx context.Context) error {
	conn, _, err := websocket.Dial(ctx, n.serverURL+"/v1/ws", nil)
	if err != nil {
		return fmt.Errorf("connecting to server: %w", err)
	}
	conn.SetReadLimit(4 << 20) // 4MB — paginated responses are ~1MB max, 4x headroom
	n.conn = conn
	n.connected.Store(true)

	go n.readLoop(ctx)
	go n.pingLoop(ctx)
	return nil
}

func (n *Network) pingLoop(ctx context.Context) {
	ticker := time.NewTicker(3 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !n.connected.Load() {
				return
			}
			pingCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			err := n.conn.Ping(pingCtx)
			cancel()
			if err != nil {
				n.connected.Store(false)
				return
			}
		}
	}
}

// Close closes the connection.
func (n *Network) Close() {
	if n.conn != nil {
		n.conn.Close(websocket.StatusNormalClosure, "bye")
		n.connected.Store(false)
	}
}

// Send sends a request and waits for the correlated response.
func (n *Network) Send(ctx context.Context, msgType protocol.MessageType, payload any) (protocol.Envelope, error) {
	reqID := fmt.Sprintf("%d", atomic.AddInt64(&n.reqSeq, 1))

	ch := make(chan protocol.Envelope, 1)
	n.mu.Lock()
	n.pending[reqID] = ch
	n.mu.Unlock()

	defer func() {
		n.mu.Lock()
		delete(n.pending, reqID)
		n.mu.Unlock()
	}()

	env := protocol.Envelope{
		Type:    msgType,
		Payload: payload,
		ReqID:   reqID,
	}

	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	n.writeMu.Lock()
	err := wsjson.Write(writeCtx, n.conn, env)
	n.writeMu.Unlock()
	if err != nil {
		return protocol.Envelope{}, fmt.Errorf("sending message: %w", err)
	}

	select {
	case resp := <-ch:
		if resp.Error != "" {
			return resp, fmt.Errorf("%s", resp.Error)
		}
		return resp, nil
	case <-ctx.Done():
		return protocol.Envelope{}, ctx.Err()
	case <-time.After(30 * time.Second):
		return protocol.Envelope{}, fmt.Errorf("request timeout")
	}
}

func (n *Network) readLoop(ctx context.Context) {
	for {
		var env protocol.Envelope
		err := wsjson.Read(ctx, n.conn, &env)
		if err != nil {
			n.connected.Store(false)
			return
		}

		if env.ReqID != "" {
			n.mu.Lock()
			ch, ok := n.pending[env.ReqID]
			n.mu.Unlock()
			if ok {
				ch <- env
				continue
			}
		}

		// Push notification
		if n.onPush != nil {
			n.onPush(env)
		}
	}
}

// --- High-level API methods ---

// Register registers a new user with the server.
func (n *Network) Register(ctx context.Context, username string, publicKey ed25519.PublicKey, homeCity string, lat, lng float64) (*protocol.RegisterResponse, error) {
	resp, err := n.Send(ctx, protocol.MsgRegister, protocol.RegisterRequest{
		Username:  username,
		PublicKey: publicKey,
		HomeCity:  homeCity,
		HomeLat:   lat,
		HomeLng:   lng,
	})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.RegisterResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// Authenticate performs the challenge-response auth flow.
func (n *Network) Authenticate(ctx context.Context, username, discriminator string, privKey ed25519.PrivateKey) (*protocol.AuthOKResponse, error) {
	// Step 1: Request challenge
	resp, err := n.Send(ctx, protocol.MsgAuth, protocol.AuthRequest{
		Username:      username,
		Discriminator: discriminator,
	})
	if err != nil {
		return nil, fmt.Errorf("auth challenge request: %w", err)
	}

	data, _ := json.Marshal(resp.Payload)
	var challenge protocol.AuthChallengeResponse
	if err := json.Unmarshal(data, &challenge); err != nil {
		return nil, fmt.Errorf("parsing challenge: %w", err)
	}

	// Step 2: Sign the nonce
	signature := ed25519.Sign(privKey, challenge.Nonce)

	// Step 3: Send signature
	resp, err = n.Send(ctx, protocol.MsgAuthResponse, protocol.AuthResponsePayload{
		Signature: signature,
	})
	if err != nil {
		return nil, fmt.Errorf("auth response: %w", err)
	}

	data, _ = json.Marshal(resp.Payload)
	var authOK protocol.AuthOKResponse
	if err := json.Unmarshal(data, &authOK); err != nil {
		return nil, fmt.Errorf("parsing auth ok: %w", err)
	}
	return &authOK, nil
}

// Recover sends a recovery request with the derived public key and returns the user record.
func (n *Network) Recover(ctx context.Context, publicKey ed25519.PublicKey) (*protocol.RecoverResponse, error) {
	resp, err := n.Send(ctx, protocol.MsgRecover, protocol.RecoverRequest{
		PublicKey: publicKey,
	})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.RecoverResponse
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, err
	}
	return &result, nil
}

// SearchCities searches for cities matching a query.
func (n *Network) SearchCities(ctx context.Context, query string) ([]protocol.CityResult, error) {
	resp, err := n.Send(ctx, protocol.MsgSearchCities, protocol.SearchCitiesRequest{
		Query: query,
		Limit: 5,
	})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.CityResultsResponse
	json.Unmarshal(data, &result)
	return result.Cities, nil
}

// GetContacts retrieves the user's contact list.
func (n *Network) GetContacts(ctx context.Context) ([]protocol.ContactItem, error) {
	resp, err := n.Send(ctx, protocol.MsgGetContacts, nil)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.ContactsResponse
	json.Unmarshal(data, &result)
	return result.Contacts, nil
}

// AddContact adds a contact by their address.
func (n *Network) AddContact(ctx context.Context, username, discriminator string) (*protocol.ContactItem, error) {
	resp, err := n.Send(ctx, protocol.MsgAddContact, protocol.AddContactRequest{
		Username:      username,
		Discriminator: discriminator,
	})
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.ContactItem
	json.Unmarshal(data, &result)
	return &result, nil
}

// GetInbox retrieves the user's inbox with cursor-based pagination.
// Pass nil for before to fetch the first page.
func (n *Network) GetInbox(ctx context.Context, before *time.Time) (*protocol.InboxResponse, error) {
	var payload any
	if before != nil {
		payload = protocol.GetInboxRequest{Before: before}
	}
	resp, err := n.Send(ctx, protocol.MsgGetInbox, payload)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.InboxResponse
	json.Unmarshal(data, &result)
	return &result, nil
}

// GetSent retrieves the user's sent mail with cursor-based pagination.
// Pass nil for before to fetch the first page.
func (n *Network) GetSent(ctx context.Context, before *time.Time) (*protocol.SentResponse, error) {
	var payload any
	if before != nil {
		payload = protocol.GetSentRequest{Before: before}
	}
	resp, err := n.Send(ctx, protocol.MsgGetSent, payload)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.SentResponse
	json.Unmarshal(data, &result)
	return &result, nil
}

// GetInTransit retrieves letters in transit to the user.
func (n *Network) GetInTransit(ctx context.Context) ([]protocol.InTransitItem, error) {
	resp, err := n.Send(ctx, protocol.MsgGetInTransit, nil)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.InTransitResponse
	json.Unmarshal(data, &result)
	return result.Letters, nil
}

// SendLetter sends an encrypted letter.
func (n *Network) SendLetter(ctx context.Context, req protocol.SendLetterRequest) (*protocol.LetterSentResponse, error) {
	resp, err := n.Send(ctx, protocol.MsgSendLetter, req)
	if err != nil {
		return nil, err
	}
	data, _ := json.Marshal(resp.Payload)
	var result protocol.LetterSentResponse
	json.Unmarshal(data, &result)
	return &result, nil
}

// SendFireAndForget writes to the WebSocket without waiting for a response.
// No ReqID is set, so any server reply falls through to the push handler.
func (n *Network) SendFireAndForget(ctx context.Context, msgType protocol.MessageType, payload any) error {
	env := protocol.Envelope{
		Type:    msgType,
		Payload: payload,
	}
	writeCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	n.writeMu.Lock()
	err := wsjson.Write(writeCtx, n.conn, env)
	n.writeMu.Unlock()
	return err
}

// MarkRead marks a message as read (fire-and-forget, no response waited).
func (n *Network) MarkRead(ctx context.Context, msgID uuid.UUID) error {
	return n.SendFireAndForget(ctx, protocol.MsgMarkRead, protocol.MarkReadRequest{MessageID: msgID})
}
