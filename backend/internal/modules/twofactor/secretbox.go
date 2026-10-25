package twofactor

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const secretboxNonceBytes = 12

func SealSecret(key []byte, plaintext string) (string, error) {
	gcm, err := newAESGCM(key)
	if err != nil {
		return "", err
	}

	nonce := make([]byte, secretboxNonceBytes)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return "", fmt.Errorf("twofactor: generate nonce: %w", err)
	}

	sealed := gcm.Seal(nonce, nonce, []byte(plaintext), nil)
	return base64.StdEncoding.EncodeToString(sealed), nil
}

func OpenSecret(key []byte, ciphertext string) (string, error) {
	gcm, err := newAESGCM(key)
	if err != nil {
		return "", err
	}

	raw, err := base64.StdEncoding.DecodeString(ciphertext)
	if err != nil {
		return "", fmt.Errorf("twofactor: decode sealed secret: %w", err)
	}
	if len(raw) < secretboxNonceBytes {
		return "", errors.New("twofactor: sealed secret is shorter than its nonce")
	}

	plaintext, err := gcm.Open(nil, raw[:secretboxNonceBytes], raw[secretboxNonceBytes:], nil)
	if err != nil {
		return "", fmt.Errorf("twofactor: open sealed secret: %w", err)
	}
	return string(plaintext), nil
}

func newAESGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("twofactor: create aes cipher: %w", err)
	}
	return cipher.NewGCM(block)
}
