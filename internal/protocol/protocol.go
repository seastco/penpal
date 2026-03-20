package protocol

import (
	"time"

	"github.com/google/uuid"
	"github.com/seastco/penpal/internal/models"
)

// MessageType identifies the type of WebSocket message.
type MessageType string

const (
	// Client -> Server
	MsgRegister        MessageType = "register"
	MsgAuth            MessageType = "auth"
	MsgAuthResponse    MessageType = "auth_response"
	MsgSendLetter      MessageType = "send_letter"
	MsgGetInbox        MessageType = "get_inbox"
	MsgGetSent         MessageType = "get_sent"
	MsgGetInTransit    MessageType = "get_in_transit"
	MsgGetTracking     MessageType = "get_tracking"
	MsgMarkRead        MessageType = "mark_read"
	MsgAddContact      MessageType = "add_contact"
	MsgGetContacts     MessageType = "get_contacts"
	MsgDeleteContact   MessageType = "delete_contact"
	MsgDeleteContactOK MessageType = "delete_contact_ok"
	MsgDeleteLetter    MessageType = "delete_letter"
	MsgDeleteLetterOK  MessageType = "delete_letter_ok"
	MsgBlockUser       MessageType = "block_user"
	MsgGetStamps       MessageType = "get_stamps"
	MsgGetMessage      MessageType = "get_message"
	MsgGetPublicKey    MessageType = "get_public_key"
	MsgSearchCities    MessageType = "search_cities"
	MsgGetShipping     MessageType = "get_shipping"
	MsgUpdateHomeCity  MessageType = "update_home_city"
	MsgRecover         MessageType = "recover"

	// Server -> Client
	MsgRegisterOK      MessageType = "register_ok"
	MsgAuthChallenge   MessageType = "auth_challenge"
	MsgAuthOK          MessageType = "auth_ok"
	MsgError           MessageType = "error"
	MsgLetterSent      MessageType = "letter_sent"
	MsgInbox           MessageType = "inbox"
	MsgSentList        MessageType = "sent_list"
	MsgInTransitList   MessageType = "in_transit_list"
	MsgTracking        MessageType = "tracking"
	MsgContactsList    MessageType = "contacts_list"
	MsgStampsList      MessageType = "stamps_list"
	MsgMessage         MessageType = "message"
	MsgPublicKey       MessageType = "public_key"
	MsgCityResults     MessageType = "city_results"
	MsgShippingInfo    MessageType = "shipping_info"
	MsgHomeCityUpdated MessageType = "home_city_updated"
	MsgRecoverOK       MessageType = "recover_ok"
)

// Envelope wraps all WebSocket messages.
type Envelope struct {
	Type    MessageType `json:"type"`
	Payload any         `json:"payload,omitempty"`
	Error   string      `json:"error,omitempty"`
	ReqID   string      `json:"req_id,omitempty"` // optional request correlation ID
}

// --- Request payloads ---

type RegisterRequest struct {
	Username  string  `json:"username"`
	PublicKey []byte  `json:"public_key"`
	HomeCity  string  `json:"home_city"`
	HomeLat   float64 `json:"home_lat"`
	HomeLng   float64 `json:"home_lng"`
}

type AuthRequest struct {
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
}

type AuthResponsePayload struct {
	Signature []byte `json:"signature"`
}

type SendLetterRequest struct {
	RecipientID   uuid.UUID   `json:"recipient_id"`
	EncryptedBody []byte      `json:"encrypted_body"`
	ShippingTier  string      `json:"shipping_tier"`
	StampIDs      []uuid.UUID `json:"stamp_ids,omitempty"`
}

type MarkReadRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type AddContactRequest struct {
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"`
	UserID        uuid.UUID `json:"user_id,omitempty"` // if set, add by ID instead of address
}

type DeleteContactRequest struct {
	ContactID uuid.UUID `json:"contact_id"`
}

type DeleteLetterRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type BlockUserRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

type GetMessageRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type GetPublicKeyRequest struct {
	UserID uuid.UUID `json:"user_id"`
}

type SearchCitiesRequest struct {
	Query string `json:"query"`
	Limit int    `json:"limit"`
}

type GetTrackingRequest struct {
	MessageID uuid.UUID `json:"message_id"`
}

type GetShippingRequest struct {
	RecipientID uuid.UUID `json:"recipient_id"`
}

type UpdateHomeCityRequest struct {
	City string  `json:"city"`
	Lat  float64 `json:"lat"`
	Lng  float64 `json:"lng"`
}

type GetInboxRequest struct {
	Before *time.Time `json:"before,omitempty"` // cursor: fetch letters delivered before this time
	Limit  int        `json:"limit,omitempty"`  // page size (default 100, max 100)
}

type GetSentRequest struct {
	Before *time.Time `json:"before,omitempty"` // cursor: fetch letters sent before this time
	Limit  int        `json:"limit,omitempty"`  // page size (default 100, max 100)
}

type RecoverRequest struct {
	PublicKey []byte `json:"public_key"`
}

// --- Response payloads ---

type RegisterResponse struct {
	UserID        uuid.UUID `json:"user_id"`
	Discriminator string    `json:"discriminator"`
}

type AuthChallengeResponse struct {
	Nonce []byte `json:"nonce"`
}

type AuthOKResponse struct {
	User models.User `json:"user"`
}

type RecoverResponse struct {
	User models.User `json:"user"`
}

type LetterSentResponse struct {
	MessageID uuid.UUID         `json:"message_id"`
	Route     []models.RouteHop `json:"route"`
	ReleaseAt time.Time         `json:"release_at"`
	Distance  float64           `json:"distance"`
}

type InboxItem struct {
	MessageID     uuid.UUID      `json:"message_id"`
	SenderName    string         `json:"sender_name"`
	SenderID      uuid.UUID      `json:"sender_id"`
	SenderPubKey  []byte         `json:"sender_pub_key"`
	EncryptedBody []byte         `json:"encrypted_body"`
	SentAt        time.Time      `json:"sent_at"`
	DeliveredAt   time.Time      `json:"delivered_at"`
	ReadAt        *time.Time     `json:"read_at,omitempty"`
	Stamps        []models.Stamp `json:"stamps,omitempty"`
}

type InboxResponse struct {
	Letters []InboxItem `json:"letters"`
	HasMore bool        `json:"has_more"`
}

type SentItem struct {
	MessageID     uuid.UUID         `json:"message_id"`
	RecipientName string            `json:"recipient_name"`
	RecipientID   uuid.UUID         `json:"recipient_id"`
	SentAt        time.Time         `json:"sent_at"`
	ShippingTier  string            `json:"shipping_tier"`
	Status        string            `json:"status"`
	Route         []models.RouteHop `json:"route"`
}

type SentResponse struct {
	Letters []SentItem `json:"letters"`
	HasMore bool       `json:"has_more"`
}

type InTransitItem struct {
	MessageID    uuid.UUID         `json:"message_id"`
	Direction    string            `json:"direction"` // "incoming" or "outgoing"
	PeerName     string            `json:"peer_name"` // sender (incoming) or recipient (outgoing)
	PeerID       uuid.UUID         `json:"peer_id"`
	OriginCity   string            `json:"origin_city"`
	DestCity     string            `json:"dest_city"`
	ShippingTier string            `json:"shipping_tier"`
	Route        []models.RouteHop `json:"route"`
	ReleaseAt    time.Time         `json:"release_at"`
}

type InTransitResponse struct {
	Letters []InTransitItem `json:"letters"`
}

type TrackingResponse struct {
	MessageID    uuid.UUID         `json:"message_id"`
	Route        []models.RouteHop `json:"route"`
	ShippingTier string            `json:"shipping_tier"`
	Status       string            `json:"status"`
	Distance     float64           `json:"distance"`
}

type ContactItem struct {
	UserID        uuid.UUID `json:"user_id"`
	Username      string    `json:"username"`
	Discriminator string    `json:"discriminator"`
	HomeCity      string    `json:"home_city"`
}

type ContactsResponse struct {
	Contacts []ContactItem `json:"contacts"`
}

type StampsResponse struct {
	Stamps []models.Stamp `json:"stamps"`
}

type GetMessageResponse struct {
	MessageID     uuid.UUID `json:"message_id"`
	SenderID      uuid.UUID `json:"sender_id"`
	EncryptedBody []byte    `json:"encrypted_body"`
}

type PublicKeyResponse struct {
	PublicKey []byte `json:"public_key"`
}

type CityResult struct {
	Name    string  `json:"name"`
	State   string  `json:"state"`
	Country string  `json:"country,omitempty"` // ISO 3166-1 alpha-2; empty = US
	Lat     float64 `json:"lat"`
	Lng     float64 `json:"lng"`
}

type CityResultsResponse struct {
	Cities []CityResult `json:"cities"`
}

type ShippingOption struct {
	Tier        string    `json:"tier"`
	Days        float64   `json:"days"`
	EstDelivery time.Time `json:"est_delivery"`
	Distance    float64   `json:"distance"`
	Hops        int       `json:"hops"`
}

type ShippingInfoResponse struct {
	FromCity string           `json:"from_city"`
	ToCity   string           `json:"to_city"`
	Options  []ShippingOption `json:"options"`
}

// Push notification payloads

type StampAwardedPush struct {
	Stamp models.Stamp `json:"stamp"`
}
