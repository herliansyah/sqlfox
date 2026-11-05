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

func TestCrypto_AADBindingPreventsFieldSwap(t *testing.T) {
	salt, _ := GenerateSalt()
	key := DeriveKey("master", salt)
	secret := []byte("s3cr3t-token")

	aadA := []byte("sqlfox:connection:aaa:password")
	aadB := []byte("sqlfox:connection:bbb:password")

	ct, err := EncryptWithAAD(secret, key, aadA)
	if err != nil {
		t.Fatalf("EncryptWithAAD failed: %v", err)
	}

	// Correct AAD decrypts.
	got, err := DecryptWithAAD(ct, key, aadA)
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("round-trip with matching AAD failed: got=%q err=%v", got, err)
	}

	// A different field/connection AAD must NOT decrypt a value sealed for another field: this is
	// the ciphertext-swap defense. (The value was sealed with AAD, so the legacy no-AAD fallback
	// also fails, as it must.)
	if _, err := DecryptWithAAD(ct, key, aadB); err == nil {
		t.Fatal("expected swap with mismatched AAD to fail, but it decrypted")
	}

	// Plain Decrypt (no AAD) must also reject an AAD-sealed value.
	if _, err := Decrypt(ct, key); err == nil {
		t.Fatal("expected no-AAD Decrypt of an AAD-sealed value to fail")
	}
}

func TestCrypto_LegacyNoAADFallback(t *testing.T) {
	salt, _ := GenerateSalt()
	key := DeriveKey("master", salt)
	secret := []byte("legacy-value")

	// Simulate a value written before AAD binding existed (sealed with nil AAD).
	legacyCT, err := Encrypt(secret, key)
	if err != nil {
		t.Fatalf("legacy Encrypt failed: %v", err)
	}

	// DecryptWithAAD must transparently fall back to no-AAD for legacy ciphertext.
	got, err := DecryptWithAAD(legacyCT, key, []byte("sqlfox:connection:xyz:password"))
	if err != nil || !bytes.Equal(got, secret) {
		t.Fatalf("legacy fallback decrypt failed: got=%q err=%v", got, err)
	}
}
