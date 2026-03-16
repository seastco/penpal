package crypto

import (
	"testing"
)

func TestGenerateMnemonic(t *testing.T) {
	mnemonic, err := GenerateMnemonic()
	if err != nil {
		t.Fatalf("GenerateMnemonic: %v", err)
	}
	// Should be 12 words
	words := splitWords(mnemonic)
	if len(words) != 12 {
		t.Fatalf("expected 12 words, got %d: %q", len(words), mnemonic)
	}
}

func TestKeypairFromMnemonic_Deterministic(t *testing.T) {
	mnemonic := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"

	pub1, priv1, err := KeypairFromMnemonic(mnemonic)
	if err != nil {
		t.Fatalf("first derivation: %v", err)
	}

	pub2, priv2, err := KeypairFromMnemonic(mnemonic)
	if err != nil {
		t.Fatalf("second derivation: %v", err)
	}

	if !pub1.Equal(pub2) {
		t.Fatal("public keys differ for same mnemonic")
	}
	if !priv1.Equal(priv2) {
		t.Fatal("private keys differ for same mnemonic")
	}
}

func TestKeypairFromMnemonic_DifferentMnemonics(t *testing.T) {
	m1 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m2 := "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"

	pub1, _, _ := KeypairFromMnemonic(m1)
	pub2, _, _ := KeypairFromMnemonic(m2)

	if pub1.Equal(pub2) {
		t.Fatal("different mnemonics should produce different keys")
	}
}

func TestKeypairFromMnemonic_InvalidMnemonic(t *testing.T) {
	_, _, err := KeypairFromMnemonic("not a valid mnemonic at all")
	if err == nil {
		t.Fatal("expected error for invalid mnemonic")
	}
}

func TestEncryptDecrypt(t *testing.T) {
	m1 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m2 := "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"

	pubA, privA, _ := KeypairFromMnemonic(m1)
	pubB, privB, _ := KeypairFromMnemonic(m2)

	plaintext := []byte("Hey Jake, how's Denver?")

	// A encrypts to B
	encrypted, err := Encrypt(plaintext, privA, pubB)
	if err != nil {
		t.Fatalf("Encrypt: %v", err)
	}

	// B decrypts from A
	decrypted, err := Decrypt(encrypted, privB, pubA)
	if err != nil {
		t.Fatalf("Decrypt: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Fatalf("decrypted text mismatch: got %q, want %q", decrypted, plaintext)
	}
}

func TestEncryptDecrypt_WrongKey(t *testing.T) {
	m1 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m2 := "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"
	m3 := "legal winner thank year wave sausage worth useful legal winner thank yellow"

	_, privA, _ := KeypairFromMnemonic(m1)
	pubB, _, _ := KeypairFromMnemonic(m2)
	_, privC, _ := KeypairFromMnemonic(m3)

	plaintext := []byte("secret message")

	// A encrypts to B
	encrypted, _ := Encrypt(plaintext, privA, pubB)

	// C tries to decrypt (should fail)
	_, err := Decrypt(encrypted, privC, pubB)
	if err == nil {
		t.Fatal("expected decryption to fail with wrong key")
	}
}

func TestVerifyKeyExchange(t *testing.T) {
	mnemonic, _ := GenerateMnemonic()
	pub, priv, _ := KeypairFromMnemonic(mnemonic)

	if err := VerifyKeyExchange(pub, priv); err != nil {
		t.Fatalf("VerifyKeyExchange: %v", err)
	}
}

func TestEncryptDecrypt_LargeMessage(t *testing.T) {
	m1 := "abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon abandon about"
	m2 := "zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo zoo wrong"

	pubA, privA, _ := KeypairFromMnemonic(m1)
	pubB, privB, _ := KeypairFromMnemonic(m2)

	// ~5000 words worth of text
	msg := make([]byte, 30000)
	for i := range msg {
		msg[i] = byte('a' + (i % 26))
	}

	encrypted, err := Encrypt(msg, privA, pubB)
	if err != nil {
		t.Fatalf("Encrypt large: %v", err)
	}

	decrypted, err := Decrypt(encrypted, privB, pubA)
	if err != nil {
		t.Fatalf("Decrypt large: %v", err)
	}

	if string(decrypted) != string(msg) {
		t.Fatal("large message roundtrip failed")
	}
}

func splitWords(s string) []string {
	var words []string
	word := ""
	for _, c := range s {
		if c == ' ' {
			if word != "" {
				words = append(words, word)
				word = ""
			}
		} else {
			word += string(c)
		}
	}
	if word != "" {
		words = append(words, word)
	}
	return words
}
