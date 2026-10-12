package auth

import (
	"encoding/base64"
	"strings"
	"testing"
)

var testSecretboxKey = []byte("0123456789abcdef0123456789abcdef")

const testSecretboxPlaintext = "JBSWY3DPEHPK3PXP"

func TestSealOpenRoundtrip(t *testing.T) {
	sealed, err := sealSecret(testSecretboxKey, testSecretboxPlaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}
	if sealed == testSecretboxPlaintext {
		t.Fatal("seal returned plaintext unchanged")
	}

	opened, err := openSecret(testSecretboxKey, sealed)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if opened != testSecretboxPlaintext {
		t.Fatalf("opened = %q, want %q", opened, testSecretboxPlaintext)
	}
}

func TestSealCiphertextIsRandomized(t *testing.T) {
	first, err := sealSecret(testSecretboxKey, testSecretboxPlaintext)
	if err != nil {
		t.Fatalf("seal first: %v", err)
	}
	second, err := sealSecret(testSecretboxKey, testSecretboxPlaintext)
	if err != nil {
		t.Fatalf("seal second: %v", err)
	}
	if first == second {
		t.Fatalf("same plaintext sealed to identical ciphertext twice: %q", first)
	}
	if strings.Contains(first, testSecretboxPlaintext) || strings.Contains(second, testSecretboxPlaintext) {
		t.Fatal("ciphertext contains plaintext")
	}

	rawFirst, err := base64.StdEncoding.DecodeString(first)
	if err != nil {
		t.Fatalf("decode first: %v", err)
	}
	rawSecond, err := base64.StdEncoding.DecodeString(second)
	if err != nil {
		t.Fatalf("decode second: %v", err)
	}
	if string(rawFirst[:12]) == string(rawSecond[:12]) {
		t.Fatal("nonce repeated across seals")
	}
}

func TestOpenRejectsTampered(t *testing.T) {
	sealed, err := sealSecret(testSecretboxKey, testSecretboxPlaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	raw, err := base64.StdEncoding.DecodeString(sealed)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	raw[len(raw)-1] ^= 0xFF

	if _, err := openSecret(testSecretboxKey, base64.StdEncoding.EncodeToString(raw)); err == nil {
		t.Fatal("expected tampered ciphertext to fail open")
	}
}

func TestOpenRejectsWrongKey(t *testing.T) {
	sealed, err := sealSecret(testSecretboxKey, testSecretboxPlaintext)
	if err != nil {
		t.Fatalf("seal: %v", err)
	}

	other := []byte("fedcba9876543210fedcba9876543210")
	if _, err := openSecret(other, sealed); err == nil {
		t.Fatal("expected wrong key to fail open")
	}
}

func TestOpenRejectsMalformedCiphertext(t *testing.T) {
	for _, value := range []string{"not-base64!!", base64.StdEncoding.EncodeToString([]byte("short"))} {
		t.Run(value, func(t *testing.T) {
			if _, err := openSecret(testSecretboxKey, value); err == nil {
				t.Fatalf("expected malformed ciphertext %q to fail open", value)
			}
		})
	}
}
