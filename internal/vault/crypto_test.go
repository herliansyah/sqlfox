package vault

import (
	"bytes"
	"testing"
)

func TestCrypto_EncryptDecrypt(t *testing.T) {
	passphrase := "super-secure-master-password"
	salt, err := GenerateSalt()
	if err != nil {
		t.Fatalf("GenerateSalt failed: %v", err)
	}

	key := DeriveKey(passphrase, salt)
	if len(key) != 32 {
		t.Fatalf("expected 32-byte key, got %d", len(key))
	}

	secretPayload := []byte("database_password_12345!@#")

	ciphertext, err := Encrypt(secretPayload, key)
	if err != nil {
		t.Fatalf("Encrypt failed: %v", err)
	}

	decrypted, err := Decrypt(ciphertext, key)
	if err != nil {
		t.Fatalf("Decrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, secretPayload) {
		t.Fatalf("expected %s, got %s", secretPayload, decrypted)
	}

	// Tamper test / wrong key
	wrongKey := DeriveKey("wrong-password", salt)
	_, err = Decrypt(ciphertext, wrongKey)
	if err == nil {
		t.Fatal("expected error with wrong key, got nil")
	}
}
