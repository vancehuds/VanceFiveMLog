package serverkeys

import (
	"strings"
	"testing"
)

func TestAPIKeyHash(t *testing.T) {
	key, hash, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if key == "" || hash == "" {
		t.Fatal("expected key and hash")
	}
	if HashAPIKey(key) != hash {
		t.Fatal("hash mismatch")
	}
}

func TestHashAPIKeyDoesNotExposePlaintext(t *testing.T) {
	key := "vfl_example"
	hash := HashAPIKey(key)
	if hash == key {
		t.Fatal("hash exposed plaintext")
	}
	if len(hash) <= len("sha256:") {
		t.Fatal("hash too short")
	}
}

func TestNewAPIKeyShape(t *testing.T) {
	key, _, err := NewAPIKey()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(key, "vfl_") {
		t.Fatalf("unexpected prefix: %s", key)
	}
	if len(key) < 40 {
		t.Fatalf("key too short: %d", len(key))
	}
}
