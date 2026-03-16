package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	pencrypto "github.com/stove/penpal/internal/crypto"
)

// Draft represents a saved letter draft.
type Draft struct {
	RecipientID    uuid.UUID `json:"recipient_id"`
	RecipientName  string    `json:"recipient_name"`
	Body           string    `json:"body"`
	OriginalMsgID  uuid.UUID `json:"original_msg_id,omitempty"`
	OriginalSender string    `json:"original_sender,omitempty"`
	SavedAt        time.Time `json:"saved_at"`
}

func draftsDir() (string, error) {
	dir, err := pencrypto.PenpalDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "drafts"), nil
}

// SaveDraft persists a draft to disk, keyed by recipient ID.
func SaveDraft(d Draft) error {
	dir, err := draftsDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	data, err := json.Marshal(d)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, d.RecipientID.String()+".json"), data, 0600)
}

// LoadDraft loads a draft for the given recipient. Returns nil, nil if no draft exists.
func LoadDraft(recipientID uuid.UUID) (*Draft, error) {
	dir, err := draftsDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, recipientID.String()+".json"))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var d Draft
	if err := json.Unmarshal(data, &d); err != nil {
		return nil, err
	}
	return &d, nil
}

// DeleteDraft removes the draft file for the given recipient. No error if it doesn't exist.
func DeleteDraft(recipientID uuid.UUID) error {
	dir, err := draftsDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, recipientID.String()+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
