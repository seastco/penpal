package models

import (
	"time"

	"github.com/google/uuid"
)

// ShippingTier determines letter delivery speed.
// Modeled after real USPS tiers, slowest to fastest.
type ShippingTier string

const (
	TierFirstClass ShippingTier = "first_class"
	TierPriority   ShippingTier = "priority"
	TierExpress    ShippingTier = "express"
)

// AllTiers returns all shipping tiers in order from slowest to fastest.
func AllTiers() []ShippingTier {
	return []ShippingTier{TierFirstClass, TierPriority, TierExpress}
}

// ValidTier returns true if the tier is a recognized shipping tier.
func ValidTier(t ShippingTier) bool {
	return t == TierFirstClass || t == TierPriority || t == TierExpress
}

// MilesPerDay returns the transit speed for this shipping tier.
func (t ShippingTier) MilesPerDay() float64 {
	switch t {
	case TierFirstClass:
		return 700
	case TierPriority:
		return 1500
	case TierExpress:
		return 2000
	default:
		return 700
	}
}

// HandlingDays returns the fixed handling overhead in days.
func (t ShippingTier) HandlingDays() float64 {
	switch t {
	case TierFirstClass:
		return 2.0
	case TierPriority:
		return 1.0
	case TierExpress:
		return 0.5
	default:
		return 2.0
	}
}

// CustomsDays returns the base customs delay in days for international mail.
// Returns 0 for domestic use — caller determines whether to apply.
func (t ShippingTier) CustomsDays() float64 {
	switch t {
	case TierFirstClass:
		return 10.0 // 7-14 day range; fat jitter tail
	case TierPriority:
		return 4.0 // 3-5 day range
	case TierExpress:
		return 1.5 // 1-2 day range
	default:
		return 10.0
	}
}

// JitterScale returns the exponential jitter scale factor.
// Higher values = more variance. Letters run late, never early.
func (t ShippingTier) JitterScale(international bool) float64 {
	if international {
		switch t {
		case TierFirstClass:
			return 0.25 // fattest tail
		case TierPriority:
			return 0.10
		case TierExpress:
			return 0.05
		default:
			return 0.25
		}
	}
	switch t {
	case TierFirstClass:
		return 0.10
	case TierPriority:
		return 0.05
	case TierExpress:
		return 0.02
	default:
		return 0.10
	}
}

// DisplayName returns the human-readable tier name.
func (t ShippingTier) DisplayName() string {
	switch t {
	case TierFirstClass:
		return "First Class"
	case TierPriority:
		return "Priority"
	case TierExpress:
		return "Express"
	default:
		return string(t)
	}
}

// MessageStatus tracks the lifecycle of a letter.
type MessageStatus string

// Message represents an encrypted letter in the system.
type Message struct {
	ID            uuid.UUID     `json:"id" db:"id"`
	SenderID      uuid.UUID     `json:"sender_id" db:"sender_id"`
	RecipientID   uuid.UUID     `json:"recipient_id" db:"recipient_id"`
	EncryptedBody []byte        `json:"encrypted_body" db:"encrypted_body"`
	ShippingTier  ShippingTier  `json:"shipping_tier" db:"shipping_tier"`
	Route         []RouteHop    `json:"route" db:"route"`
	SentAt        time.Time     `json:"sent_at" db:"sent_at"`
	ReleaseAt     time.Time     `json:"release_at" db:"release_at"`
	DeliveredAt   *time.Time    `json:"delivered_at,omitempty" db:"delivered_at"`
	ReadAt        *time.Time    `json:"read_at,omitempty" db:"read_at"`
	Status        MessageStatus `json:"status" db:"status"`
}

// RouteHop represents a single relay node in a letter's route.
type RouteHop struct {
	City  string  `json:"city"`
	Code  string  `json:"code"`
	Relay string  `json:"relay"`
	Lat   float64 `json:"lat"`
	Lng   float64 `json:"lng"`
	ETA   time.Time `json:"eta"`
}
