package crypto

import (
	"crypto/ed25519"
	"crypto/sha512"
	"fmt"

	"github.com/tyler-smith/go-bip39"
	"golang.org/x/crypto/pbkdf2"
)

const (
	// seedPBKDF2Iterations is the number of PBKDF2 iterations for deriving a key from the mnemonic.
	seedPBKDF2Iterations = 210_000
	// seedPBKDF2Salt is a fixed salt for key derivation. This is acceptable because the mnemonic
	// itself provides 128 bits of entropy.
	seedPBKDF2Salt = "penpal-ed25519-v1"
)

// GenerateMnemonic creates a new 12-word BIP39 mnemonic from 128 bits of entropy.
func GenerateMnemonic() (string, error) {
	entropy, err := bip39.NewEntropy(128)
	if err != nil {
		return "", fmt.Errorf("generating entropy: %w", err)
	}
	mnemonic, err := bip39.NewMnemonic(entropy)
	if err != nil {
		return "", fmt.Errorf("generating mnemonic: %w", err)
	}
	return mnemonic, nil
}

// KeypairFromMnemonic deterministically derives an ed25519 keypair from a BIP39 mnemonic.
// The derivation uses PBKDF2-SHA512 to produce a 32-byte seed, which is then used as the
// ed25519 private key seed.
func KeypairFromMnemonic(mnemonic string) (ed25519.PublicKey, ed25519.PrivateKey, error) {
	if !bip39.IsMnemonicValid(mnemonic) {
		return nil, nil, fmt.Errorf("invalid mnemonic")
	}
	seed := pbkdf2.Key(
		[]byte(mnemonic),
		[]byte(seedPBKDF2Salt),
		seedPBKDF2Iterations,
		ed25519.SeedSize,
		sha512.New,
	)
	privKey := ed25519.NewKeyFromSeed(seed)
	pubKey := privKey.Public().(ed25519.PublicKey)
	return pubKey, privKey, nil
}
