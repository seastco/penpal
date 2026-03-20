package server

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/protocol"
)

func TestHub_RegisterAndSend(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()

	client := &Client{
		userID: userID,
		sendCh: make(chan protocol.Envelope, 8),
	}
	hub.Register(client)

	// Should be able to send to the registered user
	hub.SendToUser(userID, "test", map[string]string{"hello": "world"})

	select {
	case env := <-client.sendCh:
		if env.Type != "test" {
			t.Fatalf("expected type 'test', got %q", env.Type)
		}
	default:
		t.Fatal("expected message on sendCh")
	}
}

func TestHub_SendToUser_Disconnected(t *testing.T) {
	hub := NewHub()

	// Sending to a user that isn't connected should not panic
	hub.SendToUser(uuid.New(), "test", nil)
}

func TestHub_Unregister(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()

	client := &Client{
		userID: userID,
		sendCh: make(chan protocol.Envelope, 8),
	}
	hub.Register(client)
	hub.Unregister(client)

	// After unregister, send should be a no-op
	hub.SendToUser(userID, "test", nil)

	select {
	case <-client.sendCh:
		t.Fatal("should not receive after unregister")
	default:
		// expected
	}
}

func TestHub_Unregister_OnlyRemovesSameClient(t *testing.T) {
	hub := NewHub()
	userID := uuid.New()

	old := &Client{userID: userID, sendCh: make(chan protocol.Envelope, 8)}
	replacement := &Client{userID: userID, sendCh: make(chan protocol.Envelope, 8)}

	// Manually set both in the hub (skip Register to avoid nil conn close)
	hub.mu.Lock()
	hub.clients[userID.String()] = old
	hub.mu.Unlock()

	hub.mu.Lock()
	hub.clients[userID.String()] = replacement
	hub.mu.Unlock()

	// Unregistering old should NOT remove replacement
	hub.Unregister(old)

	hub.SendToUser(userID, "test", nil)
	select {
	case <-replacement.sendCh:
		// good — replacement still registered
	default:
		t.Fatal("replacement should still be registered")
	}
}

func TestPickNDistinct(t *testing.T) {
	pool := []string{"a", "b", "c", "d", "e"}

	result := pickNDistinct(pool, 3)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}

	// Check all results are from pool
	poolSet := map[string]bool{"a": true, "b": true, "c": true, "d": true, "e": true}
	seen := make(map[string]bool)
	for _, r := range result {
		if !poolSet[r] {
			t.Fatalf("result %q not in pool", r)
		}
		if seen[r] {
			t.Fatalf("duplicate result: %q", r)
		}
		seen[r] = true
	}
}

func TestPickNDistinct_NGreaterThanPool(t *testing.T) {
	pool := []string{"a", "b"}
	result := pickNDistinct(pool, 10)
	if len(result) != 2 {
		t.Fatalf("expected 2 results (pool size), got %d", len(result))
	}
}

func TestStateFromCity(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"Denver, CO", "state:co"},
		{"New York, NY", "state:ny"},
		{"Boston, MA", "state:ma"},
		{"Madrid, ES", "country:es"},
		{"Lisbon, PT", "country:pt"},
		{"", ""},
		{"NoCommaHere", ""},
		{"City, ABC", ""}, // not 2-letter code
		{"City, ", ""},    // empty state
	}

	for _, tt := range tests {
		got := stateFromCity(tt.input)
		if got != tt.want {
			t.Errorf("stateFromCity(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestValidCoords(t *testing.T) {
	tests := []struct {
		lat, lng float64
		valid    bool
	}{
		{42.36, -71.06, true}, // Boston
		{0, 0, true},          // equator/prime meridian
		{-90, -180, true},     // boundary
		{90, 180, true},       // boundary
		{91, 0, false},        // lat out of range
		{0, 181, false},       // lng out of range
		{-91, 0, false},
		{0, -181, false},
	}

	for _, tt := range tests {
		got := validCoords(tt.lat, tt.lng)
		if got != tt.valid {
			t.Errorf("validCoords(%v, %v) = %v, want %v", tt.lat, tt.lng, got, tt.valid)
		}
	}
}

func TestRemoteIP_WithProxy(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"203.0.113.50, 70.41.3.18"}},
	}

	got := remoteIP(r, true)
	if got != "203.0.113.50" {
		t.Errorf("remoteIP with proxy = %q, want %q", got, "203.0.113.50")
	}
}

func TestRemoteIP_WithoutProxy(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"203.0.113.50"}},
	}

	// Should ignore X-Forwarded-For when trustProxy is false
	got := remoteIP(r, false)
	if got != "10.0.0.1" {
		t.Errorf("remoteIP without proxy = %q, want %q", got, "10.0.0.1")
	}
}

func TestRemoteIP_SingleForwardedFor(t *testing.T) {
	r := &http.Request{
		RemoteAddr: "10.0.0.1:12345",
		Header:     http.Header{"X-Forwarded-For": {"203.0.113.50"}},
	}

	got := remoteIP(r, true)
	if got != "203.0.113.50" {
		t.Errorf("remoteIP single XFF = %q, want %q", got, "203.0.113.50")
	}
}
