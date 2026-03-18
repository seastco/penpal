package crypto

import "testing"

func BenchmarkKeypairFromMnemonic(b *testing.B) {
	mnemonic, _ := GenerateMnemonic()
	b.ResetTimer()
	for b.Loop() {
		KeypairFromMnemonic(mnemonic)
	}
}

func BenchmarkEncryptDecrypt(b *testing.B) {
	mnemonic1, _ := GenerateMnemonic()
	mnemonic2, _ := GenerateMnemonic()
	pub1, priv1, _ := KeypairFromMnemonic(mnemonic1)
	pub2, _, _ := KeypairFromMnemonic(mnemonic2)
	msg := []byte("Hello, this is a benchmark message for encrypt/decrypt roundtrip.")

	_, priv2, _ := KeypairFromMnemonic(mnemonic2)

	b.ResetTimer()
	for b.Loop() {
		ct, _ := Encrypt(msg, priv1, pub2)
		Decrypt(ct, priv2, pub1)
	}
}
