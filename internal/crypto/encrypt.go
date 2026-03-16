package crypto

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha512"
	"fmt"

	"filippo.io/edwards25519"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/nacl/box"
)

// Ed25519PublicToX25519 converts an ed25519 public key to an X25519 public key
// for use with NaCl box encryption.
func Ed25519PublicToX25519(pub ed25519.PublicKey) (*[32]byte, error) {
	p, err := new(edwards25519.Point).SetBytes(pub)
	if err != nil {
		return nil, fmt.Errorf("invalid ed25519 public key: %w", err)
	}
	x25519Pub := p.BytesMontgomery()
	var result [32]byte
	copy(result[:], x25519Pub)
	return &result, nil
}

// Ed25519PrivateToX25519 converts an ed25519 private key to an X25519 private key
// for use with NaCl box encryption. Uses the standard conversion: SHA-512 hash of
// the seed, first 32 bytes clamped (same as libsodium crypto_sign_ed25519_sk_to_curve25519).
func Ed25519PrivateToX25519(priv ed25519.PrivateKey) *[32]byte {
	h := sha512.Sum512(priv.Seed())
	var x25519Priv [32]byte
	copy(x25519Priv[:], h[:32])
	x25519Priv[0] &= 248
	x25519Priv[31] &= 127
	x25519Priv[31] |= 64
	return &x25519Priv
}

// Encrypt encrypts a plaintext message from sender to recipient using NaCl box.
// Returns the nonce (24 bytes) prepended to the ciphertext.
func Encrypt(plaintext []byte, senderPriv ed25519.PrivateKey, recipientPub ed25519.PublicKey) ([]byte, error) {
	senderX := Ed25519PrivateToX25519(senderPriv)
	recipientX, err := Ed25519PublicToX25519(recipientPub)
	if err != nil {
		return nil, fmt.Errorf("converting recipient public key: %w", err)
	}

	var nonce [24]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, fmt.Errorf("generating nonce: %w", err)
	}

	sealed := box.Seal(nonce[:], plaintext, &nonce, recipientX, senderX)
	return sealed, nil
}

// Decrypt decrypts a message (nonce + ciphertext) from sender to recipient.
func Decrypt(encrypted []byte, recipientPriv ed25519.PrivateKey, senderPub ed25519.PublicKey) ([]byte, error) {
	if len(encrypted) < 24+box.Overhead {
		return nil, fmt.Errorf("encrypted message too short")
	}

	var nonce [24]byte
	copy(nonce[:], encrypted[:24])
	ciphertext := encrypted[24:]

	recipientX := Ed25519PrivateToX25519(recipientPriv)
	senderX, err := Ed25519PublicToX25519(senderPub)
	if err != nil {
		return nil, fmt.Errorf("converting sender public key: %w", err)
	}

	plaintext, ok := box.Open(nil, ciphertext, &nonce, senderX, recipientX)
	if !ok {
		return nil, fmt.Errorf("decryption failed — wrong key or corrupted message")
	}
	return plaintext, nil
}

// VerifyKeyExchange validates that ed25519->X25519 conversion works for a keypair
// by checking that the derived X25519 public key matches what curve25519.ScalarBaseMult produces.
func VerifyKeyExchange(pub ed25519.PublicKey, priv ed25519.PrivateKey) error {
	x25519Priv := Ed25519PrivateToX25519(priv)
	x25519Pub, err := Ed25519PublicToX25519(pub)
	if err != nil {
		return err
	}
	derived, err := curve25519.X25519(x25519Priv[:], curve25519.Basepoint)
	if err != nil {
		return fmt.Errorf("x25519 scalar mult: %w", err)
	}
	for i := range derived {
		if derived[i] != x25519Pub[i] {
			return fmt.Errorf("x25519 key derivation mismatch at byte %d", i)
		}
	}
	return nil
}
