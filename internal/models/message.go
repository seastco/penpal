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

// TransitSpeedMPH returns the inter-facility transit speed in miles per hour.
// First Class uses ground trucks, Priority mixes ground+air, Express is primarily air.
func (t ShippingTier) TransitSpeedMPH() float64 {
	switch t {
	case TierFirstClass:
		return 50 // ground trucks
	case TierPriority:
		return 100 // ground + some air
	case TierExpress:
		return 400 // primarily air freight
	default:
		return 50
	}
}

// DwellMeanHours returns the mean facility dwell time in hours.
// This is the dominant delay — time mail spends at each sorting facility
// (queuing, sorting, loading onto outgoing transport).
func (t ShippingTier) DwellMeanHours() float64 {
	switch t {
	case TierFirstClass:
		return 11.0
	case TierPriority:
		return 5.0
	case TierExpress:
		return 2.0
	default:
		return 11.0
	}
}

// DwellSigma returns the log-normal sigma for facility dwell time jitter.
// Higher values = more variance. Letters run late, never early.
func (t ShippingTier) DwellSigma() float64 {
	switch t {
	case TierFirstClass:
		return 0.3
	case TierPriority:
		return 0.2
	case TierExpress:
		return 0.1
	default:
		return 0.3
	}
}

// MaxFacilityHops returns the maximum number of sorting facility stops.
// Not every tracking hop is a facility — some are just transit waypoints.
func (t ShippingTier) MaxFacilityHops() int {
	switch t {
	case TierFirstClass:
		return 5
	case TierPriority:
		return 4
	case TierExpress:
		return 3
	default:
		return 5
	}
}

// RoadDetourFactor returns the multiplier to convert great-circle distance
// to estimated road/air distance.
func (t ShippingTier) RoadDetourFactor() float64 {
	switch t {
	case TierFirstClass:
		return 1.20 // ground routing
	case TierPriority:
		return 1.15 // mix
	case TierExpress:
		return 1.0 // air (great-circle is close to air route)
	default:
		return 1.20
	}
}

// IsExpress returns true if this tier operates on an express schedule
// (7 days/week, extended hours).
func (t ShippingTier) IsExpress() bool {
	return t == TierExpress
}

// CustomsDays returns the base customs delay in days for international mail.
func (t ShippingTier) CustomsDays() float64 {
	switch t {
	case TierFirstClass:
		return 10.0
	case TierPriority:
		return 4.0
	case TierExpress:
		return 1.5
	default:
		return 10.0
	}
}

// StampsRequired returns how many stamps this tier costs.
func (t ShippingTier) StampsRequired() int {
	switch t {
	case TierFirstClass:
		return 1
	case TierPriority:
		return 2
	case TierExpress:
		return 3
	default:
		return 1
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
	City  string    `json:"city"`
	Code  string    `json:"code"`
	Relay string    `json:"relay"`
	Lat   float64   `json:"lat"`
	Lng   float64   `json:"lng"`
	ETA   time.Time `json:"eta"`
}
