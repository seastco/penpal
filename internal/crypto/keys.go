package crypto

import (
	"crypto/ed25519"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// KeyFile is the on-disk format for ~/.penpal/key.
type KeyFile struct {
	PrivateKey []byte `json:"private_key"` // ed25519 64-byte private key
	PublicKey  []byte `json:"public_key"`  // ed25519 32-byte public key
}

// PenpalDir returns the path to the penpal config directory, creating it if
// necessary. Defaults to ~/.penpal but can be overridden with PENPAL_HOME.
func PenpalDir() (string, error) {
	dir := os.Getenv("PENPAL_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("getting home directory: %w", err)
		}
		dir = filepath.Join(home, ".penpal")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return "", fmt.Errorf("creating penpal directory: %w", err)
	}
	return dir, nil
}

// SaveKeyFile writes the keypair to ~/.penpal/key with restrictive permissions.
func SaveKeyFile(pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	dir, err := PenpalDir()
	if err != nil {
		return err
	}
	kf := KeyFile{
		PrivateKey: priv,
		PublicKey:  pub,
	}
	data, err := json.Marshal(kf)
	if err != nil {
		return fmt.Errorf("marshaling key file: %w", err)
	}
	path := filepath.Join(dir, "key")
	if err := os.WriteFile(path, data, 0600); err != nil {
		return fmt.Errorf("writing key file: %w", err)
	}
	return nil
}

// LoadKeyFile reads the keypair from ~/.penpal/key.
func LoadKeyFile() (ed25519.PublicKey, ed25519.PrivateKey, error) {
	dir, err := PenpalDir()
	if err != nil {
		return nil, nil, err
	}
	path := filepath.Join(dir, "key")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, fmt.Errorf("no key file found — run penpal to register")
		}
		return nil, nil, fmt.Errorf("reading key file: %w", err)
	}
	var kf KeyFile
	if err := json.Unmarshal(data, &kf); err != nil {
		return nil, nil, fmt.Errorf("parsing key file: %w", err)
	}
	if len(kf.PrivateKey) != ed25519.PrivateKeySize {
		return nil, nil, fmt.Errorf("invalid private key size: %d", len(kf.PrivateKey))
	}
	if len(kf.PublicKey) != ed25519.PublicKeySize {
		return nil, nil, fmt.Errorf("invalid public key size: %d", len(kf.PublicKey))
	}
	return ed25519.PublicKey(kf.PublicKey), ed25519.PrivateKey(kf.PrivateKey), nil
}

// KeyFileExists checks if ~/.penpal/key exists.
func KeyFileExists() bool {
	dir, err := PenpalDir()
	if err != nil {
		return false
	}
	_, err = os.Stat(filepath.Join(dir, "key"))
	return err == nil
}

// KeyPinStore maps user IDs to their previously seen public keys.
// Stored at ~/.penpal/known_keys.
type KeyPinStore map[string][]byte // userID -> public key bytes

// In-memory cache — loaded once, avoids disk I/O on every decrypt.
var (
	pinCache KeyPinStore
	pinMu    sync.Mutex
)

// loadKeyPinsLocked loads from disk if cache is empty. Caller must hold pinMu.
func loadKeyPinsLocked() (KeyPinStore, error) {
	if pinCache != nil {
		return pinCache, nil
	}
	dir, err := PenpalDir()
	if err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "known_keys")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			pinCache = make(KeyPinStore)
			return pinCache, nil
		}
		return nil, fmt.Errorf("reading known keys: %w", err)
	}
	var store KeyPinStore
	if err := json.Unmarshal(data, &store); err != nil {
		return nil, fmt.Errorf("parsing known keys: %w", err)
	}
	pinCache = store
	return pinCache, nil
}

// saveKeyPinsLocked writes the cache to disk. Caller must hold pinMu.
func saveKeyPinsLocked() error {
	dir, err := PenpalDir()
	if err != nil {
		return err
	}
	data, err := json.Marshal(pinCache)
	if err != nil {
		return fmt.Errorf("marshaling known keys: %w", err)
	}
	path := filepath.Join(dir, "known_keys")
	return os.WriteFile(path, data, 0600)
}

// ErrKeyChanged is returned when a contact's public key has changed from
// the previously pinned value. This may indicate a MITM attack.
var ErrKeyChanged = fmt.Errorf("WARNING: remote public key has changed — possible MITM attack")

// VerifyAndPinKey checks a public key against the in-memory pin cache.
// On first contact, the key is pinned (TOFU) and written to disk.
// On subsequent contacts, the check is purely in-memory (no disk I/O).
func VerifyAndPinKey(userID string, pubKey []byte) error {
	pinMu.Lock()
	defer pinMu.Unlock()

	store, err := loadKeyPinsLocked()
	if err != nil {
		return err
	}

	if pinned, exists := store[userID]; exists {
		if len(pinned) != len(pubKey) {
			return ErrKeyChanged
		}
		for i := range pinned {
			if pinned[i] != pubKey[i] {
				return ErrKeyChanged
			}
		}
		return nil // key matches pin — no disk I/O
	}

	// First contact — pin the key and flush to disk
	store[userID] = pubKey
	return saveKeyPinsLocked()
}
