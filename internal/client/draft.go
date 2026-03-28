package client

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	pencrypto "github.com/seastco/penpal/internal/crypto"
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

// draftKey returns the filename stem for a draft. Replies include the original
// message ID so that drafts for different replies to the same sender stay separate.
func draftKey(recipientID, originalMsgID uuid.UUID) string {
	if originalMsgID == uuid.Nil {
		return recipientID.String()
	}
	return recipientID.String() + "_" + originalMsgID.String()
}

// SaveDraft persists a draft to disk.
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
	return os.WriteFile(filepath.Join(dir, draftKey(d.RecipientID, d.OriginalMsgID)+".json"), data, 0600)
}

// LoadDraft loads a draft for the given recipient and original message. Returns nil, nil if no draft exists.
func LoadDraft(recipientID, originalMsgID uuid.UUID) (*Draft, error) {
	dir, err := draftsDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, draftKey(recipientID, originalMsgID)+".json"))
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

// DeleteDraft removes a draft file. No error if it doesn't exist.
func DeleteDraft(recipientID, originalMsgID uuid.UUID) error {
	dir, err := draftsDir()
	if err != nil {
		return err
	}
	err = os.Remove(filepath.Join(dir, draftKey(recipientID, originalMsgID)+".json"))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
