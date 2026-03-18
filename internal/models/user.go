package models

import (
	"time"

	"github.com/google/uuid"
)

// User represents a registered penpal user.
type User struct {
	ID            uuid.UUID `json:"id" db:"id"`
	Username      string    `json:"username" db:"username"`
	Discriminator string    `json:"discriminator" db:"discriminator"`
	PublicKey     []byte    `json:"public_key" db:"public_key"`
	HomeCity      string    `json:"home_city" db:"home_city"`
	HomeLat       float64   `json:"home_lat" db:"home_lat"`
	HomeLng       float64   `json:"home_lng" db:"home_lng"`
	LastActive    time.Time `json:"last_active" db:"last_active"`
	CreatedAt     time.Time `json:"created_at" db:"created_at"`
}
