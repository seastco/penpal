package crypto

import "testing"

func FuzzDecrypt(f *testing.F) {
	// Seed with a valid ciphertext so the fuzzer has something to mutate
	mnemonic, _ := GenerateMnemonic()
	pub, priv, _ := KeypairFromMnemonic(mnemonic)
	ct, _ := Encrypt([]byte("seed message"), priv, pub)
	f.Add(ct)
	f.Add([]byte{})
	f.Add([]byte("not a ciphertext"))

	// Generate a stable keypair for decryption attempts
	pub2, priv2, _ := KeypairFromMnemonic(mnemonic)

	f.Fuzz(func(t *testing.T, data []byte) {
		// Should never panic, only return errors
		Decrypt(data, priv2, pub2)
	})
}
