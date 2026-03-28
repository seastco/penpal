package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"math/big"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/seastco/penpal/internal/models"
)

//go:embed migrations/*.sql
var migrations embed.FS

// DB wraps the postgres connection pool.
type DB struct {
	pool *sql.DB
}

// New creates a new database connection.
func New(connStr string) (*DB, error) {
	pool, err := sql.Open("postgres", connStr)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	pool.SetMaxOpenConns(25)
	pool.SetMaxIdleConns(5)
	pool.SetConnMaxLifetime(5 * time.Minute)

	if err := pool.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}
	return &DB{pool: pool}, nil
}

// Migrate runs all embedded SQL migration files.
func (d *DB) Migrate(ctx context.Context) error {
	// Create migrations tracking table
	_, err := d.pool.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at TIMESTAMPTZ NOT NULL DEFAULT now()
		)
	`)
	if err != nil {
		return fmt.Errorf("creating migrations table: %w", err)
	}

	entries, err := migrations.ReadDir("migrations")
	if err != nil {
		return fmt.Errorf("reading migrations dir: %w", err)
	}

	for _, entry := range entries {
		name := entry.Name()
		var applied bool
		err := d.pool.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = $1)", name,
		).Scan(&applied)
		if err != nil {
			return fmt.Errorf("checking migration %s: %w", name, err)
		}
		if applied {
			continue
		}

		content, err := migrations.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("reading migration %s: %w", name, err)
		}

		tx, err := d.pool.BeginTx(ctx, nil)
		if err != nil {
			return fmt.Errorf("beginning tx for migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx, string(content)); err != nil {
			tx.Rollback()
			return fmt.Errorf("executing migration %s: %w", name, err)
		}
		if _, err := tx.ExecContext(ctx,
			"INSERT INTO schema_migrations (version) VALUES ($1)", name,
		); err != nil {
			tx.Rollback()
			return fmt.Errorf("recording migration %s: %w", name, err)
		}
		if err := tx.Commit(); err != nil {
			return fmt.Errorf("committing migration %s: %w", name, err)
		}
	}
	return nil
}

// Close closes the database connection.
func (d *DB) Close() error {
	return d.pool.Close()
}

// --- User operations ---

// generateDiscriminator creates a random 3-digit string.
func generateDiscriminator() (string, error) {
	n, err := rand.Int(rand.Reader, big.NewInt(1000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%03d", n.Int64()), nil
}

// CreateUser registers a new user, assigning a unique discriminator.
func (d *DB) CreateUser(ctx context.Context, username string, publicKey []byte, homeCity string, lat, lng float64) (*models.User, error) {
	// Try up to 10 times to find a unique discriminator
	for i := 0; i < 10; i++ {
		disc, err := generateDiscriminator()
		if err != nil {
			return nil, fmt.Errorf("generating discriminator: %w", err)
		}
		user := &models.User{
			Username:      username,
			Discriminator: disc,
			PublicKey:     publicKey,
			HomeCity:      homeCity,
			HomeLat:       lat,
			HomeLng:       lng,
		}
		err = d.pool.QueryRowContext(ctx,
			`INSERT INTO users (username, discriminator, public_key, home_city, home_lat, home_lng)
			 VALUES ($1, $2, $3, $4, $5, $6)
			 ON CONFLICT DO NOTHING
			 RETURNING id, last_active, created_at`,
			user.Username, user.Discriminator, user.PublicKey, user.HomeCity, user.HomeLat, user.HomeLng,
		).Scan(&user.ID, &user.LastActive, &user.CreatedAt)
		if err == sql.ErrNoRows {
			continue // discriminator collision, try again
		}
		if err != nil {
			return nil, fmt.Errorf("creating user: %w", err)
		}
		return user, nil
	}
	return nil, fmt.Errorf("failed to generate unique discriminator after 10 attempts")
}

// GetUserByAddress looks up a user by username#discriminator.
func (d *DB) GetUserByAddress(ctx context.Context, username, discriminator string) (*models.User, error) {
	u := &models.User{}
	err := d.pool.QueryRowContext(ctx,
		`SELECT id, username, discriminator, public_key, home_city, home_lat, home_lng, last_active, created_at
		 FROM users WHERE username = $1 AND discriminator = $2`,
		username, discriminator,
	).Scan(&u.ID, &u.Username, &u.Discriminator, &u.PublicKey, &u.HomeCity, &u.HomeLat, &u.HomeLng, &u.LastActive, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by address: %w", err)
	}
	return u, nil
}

// GetUserByID looks up a user by UUID.
func (d *DB) GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error) {
	u := &models.User{}
	err := d.pool.QueryRowContext(ctx,
		`SELECT id, username, discriminator, public_key, home_city, home_lat, home_lng, last_active, created_at
		 FROM users WHERE id = $1`, id,
	).Scan(&u.ID, &u.Username, &u.Discriminator, &u.PublicKey, &u.HomeCity, &u.HomeLat, &u.HomeLng, &u.LastActive, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by id: %w", err)
	}
	return u, nil
}

// GetUserByPublicKey looks up a user by their ed25519 public key.
func (d *DB) GetUserByPublicKey(ctx context.Context, publicKey []byte) (*models.User, error) {
	u := &models.User{}
	err := d.pool.QueryRowContext(ctx,
		`SELECT id, username, discriminator, public_key, home_city, home_lat, home_lng, last_active, created_at
		 FROM users WHERE public_key = $1`, publicKey,
	).Scan(&u.ID, &u.Username, &u.Discriminator, &u.PublicKey, &u.HomeCity, &u.HomeLat, &u.HomeLng, &u.LastActive, &u.CreatedAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by public key: %w", err)
	}
	return u, nil
}

// TouchUserActive updates the last_active timestamp for a user.
func (d *DB) TouchUserActive(ctx context.Context, userID uuid.UUID) error {
	_, err := d.pool.ExecContext(ctx,
		`UPDATE users SET last_active = now() WHERE id = $1`, userID,
	)
	return err
}

func (d *DB) UpdateHomeCity(ctx context.Context, userID uuid.UUID, city string, lat, lng float64) error {
	_, err := d.pool.ExecContext(ctx,
		`UPDATE users SET home_city = $2, home_lat = $3, home_lng = $4 WHERE id = $1`,
		userID, city, lat, lng,
	)
	return err
}

// UpdateUsername changes a user's username, preserving the discriminator if possible.
// If the new username+current discriminator collides, a new discriminator is generated.
// Returns the final discriminator.
func (d *DB) UpdateUsername(ctx context.Context, userID uuid.UUID, newUsername, currentDiscriminator string) (string, error) {
	// First try keeping the current discriminator.
	result, err := d.pool.ExecContext(ctx,
		`UPDATE users SET username = $2, discriminator = $3
		 WHERE id = $1
		 AND NOT EXISTS (
		     SELECT 1 FROM users WHERE username = $2 AND discriminator = $3 AND id != $1
		 )`,
		userID, newUsername, currentDiscriminator,
	)
	if err != nil {
		return "", fmt.Errorf("updating username: %w", err)
	}
	if n, _ := result.RowsAffected(); n == 1 {
		return currentDiscriminator, nil
	}

	// Collision: try new random discriminators up to 10 times.
	for i := 0; i < 10; i++ {
		disc, err := generateDiscriminator()
		if err != nil {
			return "", fmt.Errorf("generating discriminator: %w", err)
		}
		result, err = d.pool.ExecContext(ctx,
			`UPDATE users SET username = $2, discriminator = $3
			 WHERE id = $1
			 AND NOT EXISTS (
			     SELECT 1 FROM users WHERE username = $2 AND discriminator = $3 AND id != $1
			 )`,
			userID, newUsername, disc,
		)
		if err != nil {
			return "", fmt.Errorf("updating username: %w", err)
		}
		if n, _ := result.RowsAffected(); n == 1 {
			return disc, nil
		}
	}
	return "", fmt.Errorf("failed to find unique discriminator for username %q after 10 attempts", newUsername)
}

// --- Contact operations ---

// AddContact creates a one-directional contact relationship.
func (d *DB) AddContact(ctx context.Context, ownerID, contactID uuid.UUID) error {
	_, err := d.pool.ExecContext(ctx,
		`INSERT INTO contacts (owner_id, contact_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		ownerID, contactID,
	)
	return err
}

// GetContacts returns all contacts for a user.
func (d *DB) GetContacts(ctx context.Context, ownerID uuid.UUID) ([]models.User, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT u.id, u.username, u.discriminator, u.public_key, u.home_city, u.home_lat, u.home_lng, u.last_active, u.created_at
		 FROM contacts c JOIN users u ON c.contact_id = u.id
		 WHERE c.owner_id = $1 ORDER BY u.username LIMIT 500`, ownerID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying contacts: %w", err)
	}
	defer rows.Close()

	var contacts []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.Username, &u.Discriminator, &u.PublicKey, &u.HomeCity, &u.HomeLat, &u.HomeLng, &u.LastActive, &u.CreatedAt); err != nil {
			return nil, fmt.Errorf("scanning contact: %w", err)
		}
		contacts = append(contacts, u)
	}
	return contacts, rows.Err()
}

// DeleteContact removes a contact relationship.
func (d *DB) DeleteContact(ctx context.Context, ownerID, contactID uuid.UUID) error {
	_, err := d.pool.ExecContext(ctx,
		`DELETE FROM contacts WHERE owner_id = $1 AND contact_id = $2`,
		ownerID, contactID,
	)
	return err
}

// IsContact checks if contactID is in ownerID's contact list.
func (d *DB) IsContact(ctx context.Context, ownerID, contactID uuid.UUID) (bool, error) {
	var exists bool
	err := d.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM contacts WHERE owner_id = $1 AND contact_id = $2)`,
		ownerID, contactID,
	).Scan(&exists)
	return exists, err
}

// DeleteLetterForUser soft-deletes a message for the requesting user.
// If the user is the sender, sets sender_deleted; if recipient, sets recipient_deleted.
func (d *DB) DeleteLetterForUser(ctx context.Context, msgID, userID uuid.UUID) error {
	result, err := d.pool.ExecContext(ctx,
		`UPDATE messages
		 SET sender_deleted = CASE WHEN sender_id = $2 THEN true ELSE sender_deleted END,
		     recipient_deleted = CASE WHEN recipient_id = $2 THEN true ELSE recipient_deleted END
		 WHERE id = $1 AND (sender_id = $2 OR recipient_id = $2)`,
		msgID, userID,
	)
	if err != nil {
		return err
	}
	n, _ := result.RowsAffected()
	if n == 0 {
		return fmt.Errorf("message not found or not yours")
	}
	return nil
}

// --- Message operations ---

// CreateMessage stores a new encrypted message and returns it with the generated ID.
func (d *DB) CreateMessage(ctx context.Context, msg *models.Message, stampIDs []uuid.UUID) error {
	routeJSON, err := json.Marshal(msg.Route)
	if err != nil {
		return fmt.Errorf("marshaling route: %w", err)
	}

	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}
	defer tx.Rollback()

	err = tx.QueryRowContext(ctx,
		`INSERT INTO messages (sender_id, recipient_id, encrypted_body, shipping_tier, route, release_at)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 RETURNING id, sent_at, status`,
		msg.SenderID, msg.RecipientID, msg.EncryptedBody, msg.ShippingTier, routeJSON, msg.ReleaseAt,
	).Scan(&msg.ID, &msg.SentAt, &msg.Status)
	if err != nil {
		return fmt.Errorf("inserting message: %w", err)
	}

	// Attach stamps (atomically transfer from sender to message)
	for _, stampID := range stampIDs {
		// Verify stamp is owned by sender
		var ownerID *uuid.UUID
		err := tx.QueryRowContext(ctx,
			`SELECT owner_id FROM stamps WHERE id = $1 FOR UPDATE`, stampID,
		).Scan(&ownerID)
		if err != nil {
			return fmt.Errorf("looking up stamp %s: %w", stampID, err)
		}
		if ownerID == nil || *ownerID != msg.SenderID {
			return fmt.Errorf("stamp %s not owned by sender", stampID)
		}
		// Create attachment
		_, err = tx.ExecContext(ctx,
			`INSERT INTO stamp_attachments (message_id, stamp_id) VALUES ($1, $2)`,
			msg.ID, stampID,
		)
		if err != nil {
			return fmt.Errorf("attaching stamp: %w", err)
		}
	}

	// Consume stamps from sender (set owner_id = NULL while in transit)
	if len(stampIDs) > 0 {
		_, err = tx.ExecContext(ctx,
			`UPDATE stamps SET owner_id = NULL WHERE id = ANY($1)`,
			pq.Array(stampIDs),
		)
		if err != nil {
			return fmt.Errorf("consuming stamps: %w", err)
		}
	}

	return tx.Commit()
}

// CreateWelcomeMessage inserts an already-delivered system message (no stamps, no transaction).
func (d *DB) CreateWelcomeMessage(ctx context.Context, msg *models.Message) error {
	routeJSON, err := json.Marshal(msg.Route)
	if err != nil {
		return fmt.Errorf("marshaling route: %w", err)
	}
	now := time.Now()
	err = d.pool.QueryRowContext(ctx,
		`INSERT INTO messages (sender_id, recipient_id, encrypted_body, shipping_tier, route, release_at, delivered_at, status)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, 'delivered')
		 RETURNING id, sent_at`,
		msg.SenderID, msg.RecipientID, msg.EncryptedBody, msg.ShippingTier, routeJSON, msg.ReleaseAt, now,
	).Scan(&msg.ID, &msg.SentAt)
	if err != nil {
		return fmt.Errorf("inserting welcome message: %w", err)
	}
	msg.Status = "delivered"
	msg.DeliveredAt = &now
	return nil
}

// DeliverMessages finds all in_transit messages past their release_at and marks them delivered.
// Also transfers any attached stamps to the recipient.
func (d *DB) DeliverMessages(ctx context.Context) ([]models.Message, error) {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx,
		`UPDATE messages SET status = 'delivered', delivered_at = now()
		 WHERE status = 'in_transit' AND release_at <= now()
		 RETURNING id, sender_id, recipient_id, shipping_tier, route, sent_at, release_at, delivered_at, status`,
	)
	if err != nil {
		return nil, fmt.Errorf("delivering messages: %w", err)
	}
	defer rows.Close()

	var delivered []models.Message
	for rows.Next() {
		var m models.Message
		var routeJSON []byte
		if err := rows.Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.ShippingTier, &routeJSON, &m.SentAt, &m.ReleaseAt, &m.DeliveredAt, &m.Status); err != nil {
			return nil, fmt.Errorf("scanning delivered message: %w", err)
		}
		json.Unmarshal(routeJSON, &m.Route)
		delivered = append(delivered, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Transfer attached stamps to recipients and record discoveries
	for _, m := range delivered {
		_, err := tx.ExecContext(ctx,
			`UPDATE stamps SET owner_id = $1, earned_via = 'transfer', source_msg = $2
			 WHERE id IN (SELECT stamp_id FROM stamp_attachments WHERE message_id = $3)`,
			m.RecipientID, m.ID, m.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("transferring stamps for message %s: %w", m.ID, err)
		}
		// Record discoveries for the recipient
		_, err = tx.ExecContext(ctx,
			`INSERT INTO stamp_discoveries (user_id, stamp_type)
			 SELECT $1, s.stamp_type FROM stamps s
			 JOIN stamp_attachments sa ON sa.stamp_id = s.id
			 WHERE sa.message_id = $2
			 ON CONFLICT DO NOTHING`,
			m.RecipientID, m.ID,
		)
		if err != nil {
			return nil, fmt.Errorf("recording discoveries for message %s: %w", m.ID, err)
		}
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return delivered, nil
}

// InboxRow is an inbox message with sender info pre-joined.
type InboxRow struct {
	models.Message
	SenderName   string
	SenderPubKey []byte
}

// GetInboxWithSenders returns delivered messages with sender info, newest first.
// If before is non-nil, only returns messages delivered before that timestamp (cursor pagination).
func (d *DB) GetInboxWithSenders(ctx context.Context, userID uuid.UUID, before *time.Time, limit int) ([]InboxRow, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT m.id, m.sender_id, m.recipient_id, m.encrypted_body, m.shipping_tier, m.route,
		        m.sent_at, m.release_at, m.delivered_at, m.read_at, m.status,
		        u.username, u.public_key
		 FROM messages m
		 JOIN users u ON u.id = m.sender_id
		 WHERE m.recipient_id = $1 AND m.status IN ('delivered', 'read')
		   AND m.recipient_deleted = false
		   AND ($2::TIMESTAMPTZ IS NULL OR m.delivered_at < $2)
		 ORDER BY m.delivered_at DESC
		 LIMIT $3`, userID, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InboxRow
	for rows.Next() {
		var r InboxRow
		var routeJSON []byte
		if err := rows.Scan(&r.ID, &r.SenderID, &r.RecipientID, &r.EncryptedBody, &r.ShippingTier, &routeJSON,
			&r.SentAt, &r.ReleaseAt, &r.DeliveredAt, &r.ReadAt, &r.Status,
			&r.SenderName, &r.SenderPubKey); err != nil {
			return nil, fmt.Errorf("scanning inbox row: %w", err)
		}
		json.Unmarshal(routeJSON, &r.Route)
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetStampsForMessages returns stamps attached to any of the given message IDs, keyed by message ID.
func (d *DB) GetStampsForMessages(ctx context.Context, msgIDs []uuid.UUID) (map[uuid.UUID][]models.Stamp, error) {
	if len(msgIDs) == 0 {
		return make(map[uuid.UUID][]models.Stamp), nil
	}
	rows, err := d.pool.QueryContext(ctx,
		`SELECT s.id, s.owner_id, s.stamp_type, s.rarity, s.earned_via, s.source_msg, s.created_at,
		        sa.message_id
		 FROM stamps s
		 JOIN stamp_attachments sa ON sa.stamp_id = s.id
		 WHERE sa.message_id = ANY($1)`,
		pq.Array(msgIDs),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	result := make(map[uuid.UUID][]models.Stamp)
	for rows.Next() {
		var s models.Stamp
		var msgID uuid.UUID
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.StampType, &s.Rarity, &s.EarnedVia, &s.SourceMsg, &s.CreatedAt,
			&msgID); err != nil {
			return nil, fmt.Errorf("scanning stamp: %w", err)
		}
		result[msgID] = append(result[msgID], s)
	}
	return result, rows.Err()
}

// SentRow is a sent message with recipient name pre-joined.
type SentRow struct {
	models.Message
	RecipientName string
}

// GetSentWithRecipients returns sent messages with recipient name, newest first.
// If before is non-nil, only returns messages sent before that timestamp (cursor pagination).
func (d *DB) GetSentWithRecipients(ctx context.Context, userID uuid.UUID, before *time.Time, limit int) ([]SentRow, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT m.id, m.sender_id, m.recipient_id, m.encrypted_body, m.shipping_tier, m.route,
		        m.sent_at, m.release_at, m.delivered_at, m.read_at, m.status,
		        u.username
		 FROM messages m
		 JOIN users u ON u.id = m.recipient_id
		 WHERE m.sender_id = $1
		   AND m.sender_deleted = false
		   AND ($2::TIMESTAMPTZ IS NULL OR m.sent_at < $2)
		 ORDER BY m.sent_at DESC
		 LIMIT $3`, userID, before, limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []SentRow
	for rows.Next() {
		var r SentRow
		var routeJSON []byte
		if err := rows.Scan(&r.ID, &r.SenderID, &r.RecipientID, &r.EncryptedBody, &r.ShippingTier, &routeJSON,
			&r.SentAt, &r.ReleaseAt, &r.DeliveredAt, &r.ReadAt, &r.Status,
			&r.RecipientName); err != nil {
			return nil, fmt.Errorf("scanning sent row: %w", err)
		}
		json.Unmarshal(routeJSON, &r.Route)
		result = append(result, r)
	}
	return result, rows.Err()
}

// InTransitRow is an in-transit message with both sender and recipient info pre-joined.
type InTransitRow struct {
	ID            uuid.UUID
	SenderID      uuid.UUID
	RecipientID   uuid.UUID
	ShippingTier  models.ShippingTier
	Route         []models.RouteHop
	ReleaseAt     time.Time
	SenderName    string
	OriginCity    string
	RecipientName string
	DestCity      string
}

// GetInTransitWithUsers returns in-transit messages addressed to a user, with sender/recipient info.
func (d *DB) GetInTransitWithUsers(ctx context.Context, userID uuid.UUID) ([]InTransitRow, error) {
	return d.queryInTransitJoined(ctx,
		`WHERE m.recipient_id = $1 AND m.status = 'in_transit' ORDER BY m.release_at ASC`, userID)
}

// GetOutgoingInTransitWithUsers returns in-transit messages sent by a user, with sender/recipient info.
func (d *DB) GetOutgoingInTransitWithUsers(ctx context.Context, userID uuid.UUID) ([]InTransitRow, error) {
	return d.queryInTransitJoined(ctx,
		`WHERE m.sender_id = $1 AND m.status = 'in_transit' ORDER BY m.release_at ASC`, userID)
}

func (d *DB) queryInTransitJoined(ctx context.Context, whereClause string, userID uuid.UUID) ([]InTransitRow, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT m.id, m.sender_id, m.recipient_id, m.shipping_tier, m.route, m.release_at,
		        sender.username, sender.home_city,
		        recip.username, recip.home_city
		 FROM messages m
		 JOIN users sender ON sender.id = m.sender_id
		 JOIN users recip ON recip.id = m.recipient_id
		 `+whereClause, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []InTransitRow
	for rows.Next() {
		var r InTransitRow
		var routeJSON []byte
		if err := rows.Scan(&r.ID, &r.SenderID, &r.RecipientID, &r.ShippingTier, &routeJSON, &r.ReleaseAt,
			&r.SenderName, &r.OriginCity,
			&r.RecipientName, &r.DestCity); err != nil {
			return nil, fmt.Errorf("scanning in-transit row: %w", err)
		}
		json.Unmarshal(routeJSON, &r.Route)
		result = append(result, r)
	}
	return result, rows.Err()
}

// GetMessage returns a single message by ID.
func (d *DB) GetMessage(ctx context.Context, msgID uuid.UUID) (*models.Message, error) {
	var m models.Message
	var routeJSON []byte
	err := d.pool.QueryRowContext(ctx,
		`SELECT id, sender_id, recipient_id, encrypted_body, shipping_tier, route,
		        sent_at, release_at, delivered_at, read_at, status
		 FROM messages WHERE id = $1`, msgID,
	).Scan(&m.ID, &m.SenderID, &m.RecipientID, &m.EncryptedBody, &m.ShippingTier, &routeJSON,
		&m.SentAt, &m.ReleaseAt, &m.DeliveredAt, &m.ReadAt, &m.Status)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	json.Unmarshal(routeJSON, &m.Route)
	return &m, nil
}

// MarkRead marks a message as read.
func (d *DB) MarkRead(ctx context.Context, msgID uuid.UUID) error {
	_, err := d.pool.ExecContext(ctx,
		`UPDATE messages SET status = 'read', read_at = now() WHERE id = $1 AND status = 'delivered'`, msgID,
	)
	return err
}

// --- Rate limiting ---

// CheckRateLimit returns true if the sender is within rate limits.
func (d *DB) CheckRateLimit(ctx context.Context, senderID, recipientID uuid.UUID) (bool, error) {
	// Per sender-recipient: max 10 per day
	var pairCount int
	err := d.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages
		 WHERE sender_id = $1 AND recipient_id = $2 AND sent_at > now() - interval '1 day'`,
		senderID, recipientID,
	).Scan(&pairCount)
	if err != nil {
		return false, err
	}
	if pairCount >= 10 {
		return false, nil
	}

	// Per sender total: max 50 per day
	var totalCount int
	err = d.pool.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM messages
		 WHERE sender_id = $1 AND sent_at > now() - interval '1 day'`,
		senderID,
	).Scan(&totalCount)
	if err != nil {
		return false, err
	}
	return totalCount < 50, nil
}

// IsBlocked checks if blockerID has blocked blockedID.
func (d *DB) IsBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error) {
	var exists bool
	err := d.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM blocks WHERE blocker_id = $1 AND blocked_id = $2)`,
		blockerID, blockedID,
	).Scan(&exists)
	return exists, err
}

// BlockUser adds a block relationship and removes the contact atomically.
func (d *DB) BlockUser(ctx context.Context, blockerID, blockedID uuid.UUID) error {
	tx, err := d.pool.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.ExecContext(ctx,
		`INSERT INTO blocks (blocker_id, blocked_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		blockerID, blockedID,
	); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM contacts WHERE owner_id = $1 AND contact_id = $2`,
		blockerID, blockedID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Stamp operations ---

// GetStamps returns all stamps owned by a user.
func (d *DB) GetStamps(ctx context.Context, ownerID uuid.UUID) ([]models.Stamp, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT id, owner_id, stamp_type, rarity, earned_via, source_msg, created_at
		 FROM stamps WHERE owner_id = $1 ORDER BY created_at LIMIT 1000`, ownerID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var stamps []models.Stamp
	for rows.Next() {
		var s models.Stamp
		if err := rows.Scan(&s.ID, &s.OwnerID, &s.StampType, &s.Rarity, &s.EarnedVia, &s.SourceMsg, &s.CreatedAt); err != nil {
			return nil, err
		}
		stamps = append(stamps, s)
	}
	return stamps, rows.Err()
}

// CreateStamp creates a new stamp for a user.
func (d *DB) CreateStamp(ctx context.Context, ownerID uuid.UUID, stampType string, rarity models.StampRarity, earnedVia models.EarnedVia) (*models.Stamp, error) {
	s := &models.Stamp{
		OwnerID:   &ownerID,
		StampType: stampType,
		Rarity:    rarity,
		EarnedVia: earnedVia,
	}
	err := d.pool.QueryRowContext(ctx,
		`INSERT INTO stamps (owner_id, stamp_type, rarity, earned_via) VALUES ($1, $2, $3, $4)
		 RETURNING id, created_at`,
		s.OwnerID, s.StampType, s.Rarity, s.EarnedVia,
	).Scan(&s.ID, &s.CreatedAt)
	if err != nil {
		return nil, err
	}
	_ = d.RecordDiscovery(ctx, ownerID, stampType)
	return s, nil
}

// RecordDiscovery records that a user has discovered a stamp type (idempotent).
func (d *DB) RecordDiscovery(ctx context.Context, userID uuid.UUID, stampType string) error {
	_, err := d.pool.ExecContext(ctx,
		`INSERT INTO stamp_discoveries (user_id, stamp_type) VALUES ($1, $2) ON CONFLICT DO NOTHING`,
		userID, stampType,
	)
	return err
}

// GetDiscoveries returns all stamp types a user has ever owned.
func (d *DB) GetDiscoveries(ctx context.Context, userID uuid.UUID) ([]string, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT stamp_type FROM stamp_discoveries WHERE user_id = $1`, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var types []string
	for rows.Next() {
		var t string
		if err := rows.Scan(&t); err != nil {
			return nil, err
		}
		types = append(types, t)
	}
	return types, rows.Err()
}

// HasReceivedFrom checks if userID has received any delivered/read letter from senderID.
func (d *DB) HasReceivedFrom(ctx context.Context, userID, senderID uuid.UUID) (bool, error) {
	var exists bool
	err := d.pool.QueryRowContext(ctx,
		`SELECT EXISTS(SELECT 1 FROM messages
		 WHERE recipient_id = $1 AND sender_id = $2
		 AND status IN ('delivered', 'read'))`,
		userID, senderID,
	).Scan(&exists)
	return exists, err
}

// GetLastWeeklyStampTime returns the time of the most recent weekly stamp for a user.
// Returns epoch (1970-01-01) if no weekly stamp exists, which always triggers a first award.
func (d *DB) GetLastWeeklyStampTime(ctx context.Context, ownerID uuid.UUID) (time.Time, error) {
	var t time.Time
	err := d.pool.QueryRowContext(ctx,
		`SELECT COALESCE(
		     (SELECT MAX(s.created_at) FROM stamps s WHERE s.owner_id = $1 AND s.earned_via = 'weekly'),
		     (SELECT created_at FROM users WHERE id = $1)
		 )`,
		ownerID,
	).Scan(&t)
	return t, err
}

// WeeklyStampUser holds the minimal user info needed for weekly stamp awards.
type WeeklyStampUser struct {
	ID       uuid.UUID
	HomeCity string
}

// GetUsersNeedingWeeklyStamp returns users who haven't received a weekly stamp in 7+ days.
func (d *DB) GetUsersNeedingWeeklyStamp(ctx context.Context) ([]WeeklyStampUser, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT u.id, u.home_city FROM users u
		 WHERE u.created_at <= now() - interval '7 days'
		 AND NOT EXISTS (
		     SELECT 1 FROM stamps s
		     WHERE s.owner_id = u.id
		     AND s.earned_via = 'weekly'
		     AND s.created_at > now() - interval '7 days'
		 )
		 LIMIT 500`,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []WeeklyStampUser
	for rows.Next() {
		var u WeeklyStampUser
		if err := rows.Scan(&u.ID, &u.HomeCity); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

// ReapGhostAccounts deletes users who have never sent or received a message
// and whose last_active is before the given cutoff. Returns the number deleted.
func (d *DB) ReapGhostAccounts(ctx context.Context, inactiveBefore time.Time) (int64, error) {
	result, err := d.pool.ExecContext(ctx,
		`DELETE FROM users
		 WHERE last_active < $1
		   AND NOT EXISTS (SELECT 1 FROM messages WHERE sender_id = users.id)
		   AND NOT EXISTS (SELECT 1 FROM messages WHERE recipient_id = users.id)`,
		inactiveBefore,
	)
	if err != nil {
		return 0, fmt.Errorf("reaping ghost accounts: %w", err)
	}
	return result.RowsAffected()
}

// UserAddress is a username#discriminator pair.
type UserAddress struct {
	Username      string
	Discriminator string
}

// GetAllUsers returns all registered user addresses.
func (d *DB) GetAllUsers(ctx context.Context) ([]UserAddress, error) {
	rows, err := d.pool.QueryContext(ctx,
		`SELECT username, discriminator FROM users ORDER BY username, discriminator`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []UserAddress
	for rows.Next() {
		var u UserAddress
		if err := rows.Scan(&u.Username, &u.Discriminator); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}
