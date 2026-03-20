package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/models"
)

// Store defines the interface for all database operations.
// The concrete *DB type satisfies this interface.
type Store interface {
	Migrate(ctx context.Context) error
	Close() error

	// User operations
	CreateUser(ctx context.Context, username string, publicKey []byte, homeCity string, lat, lng float64) (*models.User, error)
	GetUserByAddress(ctx context.Context, username, discriminator string) (*models.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*models.User, error)
	GetUserByPublicKey(ctx context.Context, publicKey []byte) (*models.User, error)
	TouchUserActive(ctx context.Context, userID uuid.UUID) error
	UpdateHomeCity(ctx context.Context, userID uuid.UUID, city string, lat, lng float64) error

	// Contact operations
	AddContact(ctx context.Context, ownerID, contactID uuid.UUID) error
	GetContacts(ctx context.Context, ownerID uuid.UUID) ([]models.User, error)
	DeleteContact(ctx context.Context, ownerID, contactID uuid.UUID) error
	IsContact(ctx context.Context, ownerID, contactID uuid.UUID) (bool, error)

	// Message operations
	CreateMessage(ctx context.Context, msg *models.Message, stampIDs []uuid.UUID) error
	DeliverMessages(ctx context.Context) ([]models.Message, error)
	GetInboxWithSenders(ctx context.Context, userID uuid.UUID, before *time.Time, limit int) ([]InboxRow, error)
	GetStampsForMessages(ctx context.Context, msgIDs []uuid.UUID) (map[uuid.UUID][]models.Stamp, error)
	GetSentWithRecipients(ctx context.Context, userID uuid.UUID, before *time.Time, limit int) ([]SentRow, error)
	GetInTransitWithUsers(ctx context.Context, userID uuid.UUID) ([]InTransitRow, error)
	GetOutgoingInTransitWithUsers(ctx context.Context, userID uuid.UUID) ([]InTransitRow, error)
	GetMessage(ctx context.Context, msgID uuid.UUID) (*models.Message, error)
	MarkRead(ctx context.Context, msgID uuid.UUID) error
	DeleteLetterForUser(ctx context.Context, msgID, userID uuid.UUID) error

	// Rate limiting and blocking
	CheckRateLimit(ctx context.Context, senderID, recipientID uuid.UUID) (bool, error)
	IsBlocked(ctx context.Context, blockerID, blockedID uuid.UUID) (bool, error)
	BlockUser(ctx context.Context, blockerID, blockedID uuid.UUID) error

	// Stamp operations
	GetStamps(ctx context.Context, ownerID uuid.UUID) ([]models.Stamp, error)
	CreateStamp(ctx context.Context, ownerID uuid.UUID, stampType string, rarity models.StampRarity, earnedVia models.EarnedVia) (*models.Stamp, error)
	HasReceivedFrom(ctx context.Context, userID, senderID uuid.UUID) (bool, error)
	GetLastWeeklyStampTime(ctx context.Context, ownerID uuid.UUID) (time.Time, error)
	GetUsersNeedingWeeklyStamp(ctx context.Context) ([]WeeklyStampUser, error)

	// System messages
	CreateWelcomeMessage(ctx context.Context, msg *models.Message) error

	// Maintenance
	ReapGhostAccounts(ctx context.Context, inactiveBefore time.Time) (int64, error)
	GetAllUsers(ctx context.Context) ([]UserAddress, error)
}
