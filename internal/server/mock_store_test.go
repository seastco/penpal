package server

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/db"
	"github.com/seastco/penpal/internal/models"
)

// mockStore is an in-memory implementation of db.Store for testing.
type mockStore struct {
	mu       sync.Mutex
	users    map[uuid.UUID]*models.User
	contacts map[uuid.UUID]map[uuid.UUID]bool // ownerID -> contactIDs
	blocks   map[uuid.UUID]map[uuid.UUID]bool // blockerID -> blockedIDs
	messages map[uuid.UUID]*models.Message
	stamps   map[uuid.UUID]*models.Stamp
	// stampAttachments tracks which stamps are attached to which messages
	stampAttachments map[uuid.UUID][]uuid.UUID // messageID -> stampIDs

	// configurable behavior for tests
	rateLimitOK bool
}

func newMockStore() *mockStore {
	return &mockStore{
		users:            make(map[uuid.UUID]*models.User),
		contacts:         make(map[uuid.UUID]map[uuid.UUID]bool),
		blocks:           make(map[uuid.UUID]map[uuid.UUID]bool),
		messages:         make(map[uuid.UUID]*models.Message),
		stamps:           make(map[uuid.UUID]*models.Stamp),
		stampAttachments: make(map[uuid.UUID][]uuid.UUID),
		rateLimitOK:      true,
	}
}

func (m *mockStore) Migrate(_ context.Context) error { return nil }
func (m *mockStore) Close() error                    { return nil }

func (m *mockStore) CreateUser(_ context.Context, username string, publicKey []byte, homeCity string, lat, lng float64) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	user := &models.User{
		ID:            uuid.New(),
		Username:      username,
		Discriminator: fmt.Sprintf("%04d", len(m.users)),
		PublicKey:     publicKey,
		HomeCity:      homeCity,
		HomeLat:       lat,
		HomeLng:       lng,
		LastActive:    time.Now(),
		CreatedAt:     time.Now(),
	}
	m.users[user.ID] = user
	return user, nil
}

func (m *mockStore) GetUserByAddress(_ context.Context, username, discriminator string) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, u := range m.users {
		if u.Username == username && u.Discriminator == discriminator {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockStore) GetUserByID(_ context.Context, id uuid.UUID) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if u, ok := m.users[id]; ok {
		return u, nil
	}
	return nil, nil
}

func (m *mockStore) GetUserByPublicKey(_ context.Context, publicKey []byte) (*models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, u := range m.users {
		if string(u.PublicKey) == string(publicKey) {
			return u, nil
		}
	}
	return nil, nil
}

func (m *mockStore) TouchUserActive(_ context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if u, ok := m.users[userID]; ok {
		u.LastActive = time.Now()
	}
	return nil
}

func (m *mockStore) UpdateHomeCity(_ context.Context, userID uuid.UUID, city string, lat, lng float64) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if u, ok := m.users[userID]; ok {
		u.HomeCity = city
		u.HomeLat = lat
		u.HomeLng = lng
	}
	return nil
}

func (m *mockStore) AddContact(_ context.Context, ownerID, contactID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.contacts[ownerID] == nil {
		m.contacts[ownerID] = make(map[uuid.UUID]bool)
	}
	m.contacts[ownerID][contactID] = true
	return nil
}

func (m *mockStore) GetContacts(_ context.Context, ownerID uuid.UUID) ([]models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []models.User
	for cid := range m.contacts[ownerID] {
		if u, ok := m.users[cid]; ok {
			result = append(result, *u)
		}
	}
	return result, nil
}

func (m *mockStore) DeleteContact(_ context.Context, ownerID, contactID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.contacts[ownerID]; ok {
		delete(c, contactID)
	}
	return nil
}

func (m *mockStore) IsContact(_ context.Context, ownerID, contactID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if c, ok := m.contacts[ownerID]; ok {
		return c[contactID], nil
	}
	return false, nil
}

func (m *mockStore) CreateMessage(_ context.Context, msg *models.Message, stampIDs []uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg.ID = uuid.New()
	msg.SentAt = time.Now()
	msg.Status = "in_transit"
	m.messages[msg.ID] = msg
	m.stampAttachments[msg.ID] = stampIDs
	return nil
}

func (m *mockStore) CreateWelcomeMessage(_ context.Context, msg *models.Message) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	msg.ID = uuid.New()
	msg.SentAt = time.Now()
	msg.Status = "delivered"
	now := time.Now()
	msg.DeliveredAt = &now
	m.messages[msg.ID] = msg
	return nil
}

func (m *mockStore) DeliverMessages(_ context.Context) ([]models.Message, error) {
	return nil, nil
}

func (m *mockStore) GetInboxWithSenders(_ context.Context, userID uuid.UUID, _ *time.Time, limit int) ([]db.InboxRow, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var rows []db.InboxRow
	for _, msg := range m.messages {
		if msg.RecipientID == userID && (msg.Status == "delivered" || msg.Status == "read") {
			sender := m.users[msg.SenderID]
			row := db.InboxRow{Message: *msg}
			if sender != nil {
				row.SenderName = sender.Username
				row.SenderPubKey = sender.PublicKey
			}
			rows = append(rows, row)
			if len(rows) >= limit {
				break
			}
		}
	}
	return rows, nil
}

func (m *mockStore) GetStampsForMessages(_ context.Context, msgIDs []uuid.UUID) (map[uuid.UUID][]models.Stamp, error) {
	return nil, nil
}

func (m *mockStore) GetSentWithRecipients(_ context.Context, userID uuid.UUID, _ *time.Time, limit int) ([]db.SentRow, error) {
	return nil, nil
}

func (m *mockStore) GetInTransitWithUsers(_ context.Context, _ uuid.UUID) ([]db.InTransitRow, error) {
	return nil, nil
}

func (m *mockStore) GetOutgoingInTransitWithUsers(_ context.Context, _ uuid.UUID) ([]db.InTransitRow, error) {
	return nil, nil
}

func (m *mockStore) GetMessage(_ context.Context, msgID uuid.UUID) (*models.Message, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg, ok := m.messages[msgID]; ok {
		return msg, nil
	}
	return nil, nil
}

func (m *mockStore) DeleteLetterForUser(_ context.Context, msgID, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.messages[msgID]; ok {
		delete(m.messages, msgID)
	}
	return nil
}

func (m *mockStore) MarkRead(_ context.Context, msgID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if msg, ok := m.messages[msgID]; ok {
		msg.Status = "read"
		now := time.Now()
		msg.ReadAt = &now
	}
	return nil
}

func (m *mockStore) CheckRateLimit(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return m.rateLimitOK, nil
}

func (m *mockStore) IsBlocked(_ context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	if b, ok := m.blocks[blockerID]; ok {
		return b[blockedID], nil
	}
	return false, nil
}

func (m *mockStore) BlockUser(_ context.Context, blockerID, blockedID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if m.blocks[blockerID] == nil {
		m.blocks[blockerID] = make(map[uuid.UUID]bool)
	}
	m.blocks[blockerID][blockedID] = true
	return nil
}

func (m *mockStore) GetStamps(_ context.Context, ownerID uuid.UUID) ([]models.Stamp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []models.Stamp
	for _, s := range m.stamps {
		if s.OwnerID != nil && *s.OwnerID == ownerID {
			result = append(result, *s)
		}
	}
	return result, nil
}

func (m *mockStore) CreateStamp(_ context.Context, ownerID uuid.UUID, stampType string, rarity models.StampRarity, earnedVia models.EarnedVia) (*models.Stamp, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	s := &models.Stamp{
		ID:        uuid.New(),
		OwnerID:   &ownerID,
		StampType: stampType,
		Rarity:    rarity,
		EarnedVia: earnedVia,
		CreatedAt: time.Now(),
	}
	m.stamps[s.ID] = s
	return s, nil
}

func (m *mockStore) HasReceivedFrom(_ context.Context, _, _ uuid.UUID) (bool, error) {
	return false, nil
}

func (m *mockStore) GetLastWeeklyStampTime(_ context.Context, _ uuid.UUID) (time.Time, error) {
	return time.Time{}, nil
}

func (m *mockStore) GetUsersNeedingWeeklyStamp(_ context.Context) ([]db.WeeklyStampUser, error) {
	return nil, nil
}

func (m *mockStore) ReapGhostAccounts(_ context.Context, _ time.Time) (int64, error) {
	return 0, nil
}

func (m *mockStore) GetAllUsers(_ context.Context) ([]db.UserAddress, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var result []db.UserAddress
	for _, u := range m.users {
		result = append(result, db.UserAddress{Username: u.Username, Discriminator: u.Discriminator})
	}
	return result, nil
}

// addUser is a test helper to seed a user into the mock store.
func (m *mockStore) addUser(u *models.User) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.users[u.ID] = u
}
