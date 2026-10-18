package secrets

import (
	"errors"
	"testing"
)

func TestSealOpen(t *testing.T) {
	box := New("a-secret-that-is-at-least-32-characters")
	sealed, err := box.Seal([]byte("sk-ant-123"))
	if err != nil {
		t.Fatal(err)
	}
	plain, err := box.Open(sealed)
	if err != nil || string(plain) != "sk-ant-123" {
		t.Fatalf("Open = %q, %v", plain, err)
	}
	if _, err := New("another-secret-that-is-32-characters-long").Open(sealed); err == nil {
		t.Fatal("a different key must not open the value")
	}
	if _, err := New("").Seal([]byte("x")); !errors.Is(err, ErrNoKey) {
		t.Fatalf("empty key: %v", err)
	}
	if Hint("sk-ant-api03-abcdefgh") != "••••efgh" {
		t.Fatal("hint shows the last four characters")
	}
}
