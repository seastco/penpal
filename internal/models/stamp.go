package models

import (
	"time"

	"github.com/google/uuid"
)

// StampRarity indicates how rare a stamp is.
type StampRarity string

const (
	RarityCommon StampRarity = "common"
	RarityRare   StampRarity = "rare"
	RarityUltra  StampRarity = "ultra_rare"
)

// EarnedVia describes how a stamp was acquired.
type EarnedVia string

const (
	EarnedRegistration EarnedVia = "registration"
	EarnedWeekly       EarnedVia = "weekly"
)

// Stamp represents a collectible stamp owned by a user.
type Stamp struct {
	ID        uuid.UUID   `json:"id" db:"id"`
	OwnerID   *uuid.UUID  `json:"owner_id,omitempty" db:"owner_id"`
	StampType string      `json:"stamp_type" db:"stamp_type"`
	Rarity    StampRarity `json:"rarity" db:"rarity"`
	EarnedVia EarnedVia   `json:"earned_via" db:"earned_via"`
	SourceMsg *uuid.UUID  `json:"source_msg,omitempty" db:"source_msg"`
	CreatedAt time.Time   `json:"created_at" db:"created_at"`
}

