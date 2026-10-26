package passwordreset

import (
	"crypto/sha256"
	"encoding/hex"
	"testing"
)

func TestNewTokenShape(t *testing.T) {
	raw, hash, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if len(raw) != 43 {
		t.Fatalf("raw length = %d, want 43", len(raw))
	}
	if len(hash) != 64 {
		t.Fatalf("hash length = %d, want 64", len(hash))
	}
	if hash != hashToken(raw) {
		t.Fatal("returned hash does not match hashToken")
	}
	raw2, _, err := newToken()
	if err != nil {
		t.Fatalf("newToken: %v", err)
	}
	if raw == raw2 {
		t.Fatal("two tokens are identical")
	}
}

func TestHashTokenIsSHA256Hex(t *testing.T) {
	sum := sha256.Sum256([]byte("abc"))
	if got := hashToken("abc"); got != hex.EncodeToString(sum[:]) {
		t.Fatalf("hashToken(abc) = %q", got)
	}
}
