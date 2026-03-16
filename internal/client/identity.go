package client

import (
	"encoding/json"
	"os"
	"path/filepath"

	pencrypto "github.com/stove/penpal/internal/crypto"
)

// identityFile stores username and discriminator alongside the key.
type identityFile struct {
	Username      string `json:"username"`
	Discriminator string `json:"discriminator"`
}

func saveIdentity(username, discriminator string) error {
	dir, err := pencrypto.PenpalDir()
	if err != nil {
		return err
	}
	data, _ := json.Marshal(identityFile{
		Username:      username,
		Discriminator: discriminator,
	})
	return os.WriteFile(filepath.Join(dir, "identity"), data, 0600)
}

// LoadIdentityPublic loads the saved username and discriminator from disk.
func LoadIdentityPublic() (string, string, error) {
	return loadIdentity()
}

func loadIdentity() (string, string, error) {
	dir, err := pencrypto.PenpalDir()
	if err != nil {
		return "", "", err
	}
	data, err := os.ReadFile(filepath.Join(dir, "identity"))
	if err != nil {
		return "", "", err
	}
	var id identityFile
	if err := json.Unmarshal(data, &id); err != nil {
		return "", "", err
	}
	return id.Username, id.Discriminator, nil
}
