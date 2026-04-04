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
	EarnedMint         EarnedVia = "mint"
)

// ValidStampTypes contains all mintable stamp types and their rarities.
var ValidStampTypes = map[string]StampRarity{
	// Common
	"common:flag": RarityCommon, "common:heart": RarityCommon, "common:star": RarityCommon,
	"common:quill": RarityCommon, "common:blossom": RarityCommon, "common:sunflower": RarityCommon,
	"common:butterfly": RarityCommon, "common:wave": RarityCommon, "common:moon": RarityCommon,
	"common:bird": RarityCommon, "common:rainbow": RarityCommon, "common:clover": RarityCommon,
	"common:envelope": RarityCommon, "common:seal": RarityCommon, "common:horn": RarityCommon,
	"common:scroll": RarityCommon, "common:mushroom": RarityCommon, "common:leaf": RarityCommon,
	"common:shell": RarityCommon, "common:pine": RarityCommon, "common:owl": RarityCommon,
	"common:fox": RarityCommon, "common:bee": RarityCommon, "common:turtle": RarityCommon,
	// States
	"state:ak": RarityCommon, "state:al": RarityCommon, "state:ar": RarityCommon, "state:az": RarityCommon,
	"state:ca": RarityCommon, "state:co": RarityCommon, "state:ct": RarityCommon, "state:de": RarityCommon,
	"state:fl": RarityCommon, "state:ga": RarityCommon, "state:hi": RarityCommon, "state:ia": RarityCommon,
	"state:id": RarityCommon, "state:il": RarityCommon, "state:in": RarityCommon, "state:ks": RarityCommon,
	"state:ky": RarityCommon, "state:la": RarityCommon, "state:ma": RarityCommon, "state:md": RarityCommon,
	"state:me": RarityCommon, "state:mi": RarityCommon, "state:mn": RarityCommon, "state:mo": RarityCommon,
	"state:ms": RarityCommon, "state:mt": RarityCommon, "state:nc": RarityCommon, "state:nd": RarityCommon,
	"state:ne": RarityCommon, "state:nh": RarityCommon, "state:nj": RarityCommon, "state:nm": RarityCommon,
	"state:nv": RarityCommon, "state:ny": RarityCommon, "state:oh": RarityCommon, "state:ok": RarityCommon,
	"state:or": RarityCommon, "state:pa": RarityCommon, "state:ri": RarityCommon, "state:sc": RarityCommon,
	"state:sd": RarityCommon, "state:tn": RarityCommon, "state:tx": RarityCommon, "state:ut": RarityCommon,
	"state:va": RarityCommon, "state:vt": RarityCommon, "state:wa": RarityCommon, "state:wi": RarityCommon,
	"state:wv": RarityCommon, "state:wy": RarityCommon,
	// Rare
	"rare:cross_country": RarityRare, "rare:explorer": RarityRare, "rare:penpal": RarityRare,
	"rare:faithful": RarityRare, "rare:collector": RarityRare,
}

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
