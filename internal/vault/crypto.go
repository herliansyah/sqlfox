package vault

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
)

const (
	argonTime    = 3
	argonMemory  = 64 * 1024 // 64 MB
	argonThreads = 4
	argonKeyLen  = 32        // 256 bits
	saltLen      = 16
	nonceLen     = 12
)

var (
	ErrInvalidCiphertext = errors.New("invalid or corrupted ciphertext")
	ErrDecryptionFailed  = errors.New("decryption failed: incorrect key or corrupted data")
)

// GenerateSalt produces cryptographically random bytes for key derivation.
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, saltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, fmt.Errorf("failed to generate salt: %w", err)
	}
	return salt, nil
}

// DeriveKey derives a 256-bit AES key from a passphrase and salt using Argon2id.
func DeriveKey(passphrase string, salt []byte) []byte {
	return argon2.IDKey([]byte(passphrase), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
}

// Encrypt encrypts plaintext bytes with AES-256-GCM and returns a base64 encoded string.
func Encrypt(plaintext []byte, key []byte) (string, error) {
	if len(key) != argonKeyLen {
		return "", fmt.Errorf("invalid key length: expected %d bytes, got %d", argonKeyLen, len(key))
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("failed to initialize GCM: %w", err)
	}

	nonce := make([]byte, gcm.NonceSize())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Seal appends the ciphertext and tag to nonce
	ciphertext := gcm.Seal(nonce, nonce, plaintext, nil)
	return base64.StdEncoding.EncodeToString(ciphertext), nil
}

// Decrypt decodes a base64 string and decrypts it using AES-256-GCM.
func Decrypt(encodedCiphertext string, key []byte) ([]byte, error) {
	if len(key) != argonKeyLen {
		return nil, fmt.Errorf("invalid key length: expected %d bytes, got %d", argonKeyLen, len(key))
	}

	data, err := base64.StdEncoding.DecodeString(encodedCiphertext)
	if err != nil {
		return nil, ErrInvalidCiphertext
	}

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize GCM: %w", err)
	}

	if len(data) < gcm.NonceSize() {
		return nil, ErrInvalidCiphertext
	}

	nonce := data[:gcm.NonceSize()]
	actualCiphertext := data[gcm.NonceSize():]

	plaintext, err := gcm.Open(nil, nonce, actualCiphertext, nil)
	if err != nil {
		return nil, ErrDecryptionFailed
	}

	return plaintext, nil
}
