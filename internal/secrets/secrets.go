// Package secrets seals small values (API keys, portal cookies) before they
// are stored in Postgres, so a database dump alone does not leak them.
package secrets

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
)

var ErrNoKey = errors.New("JOBS_SECRET_KEY is not set, so stored secrets can't be read or written")

type Box struct{ aead cipher.AEAD }

// New derives an AES-256-GCM key from secret. An empty secret gives a Box
// whose every call fails with ErrNoKey, so the rest of the app still runs.
func New(secret string) *Box {
	if secret == "" {
		return &Box{}
	}
	key := sha256.Sum256([]byte("portfolio/jobs/v1:" + secret))
	block, err := aes.NewCipher(key[:])
	if err != nil {
		panic(err) // a 32-byte key never fails
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		panic(err)
	}
	return &Box{aead: aead}
}

func (b *Box) Ready() bool { return b != nil && b.aead != nil }

// Seal returns nonce||ciphertext.
func (b *Box) Seal(plain []byte) ([]byte, error) {
	if !b.Ready() {
		return nil, ErrNoKey
	}
	nonce := make([]byte, b.aead.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return b.aead.Seal(nonce, nonce, plain, nil), nil
}

func (b *Box) Open(sealed []byte) ([]byte, error) {
	if !b.Ready() {
		return nil, ErrNoKey
	}
	n := b.aead.NonceSize()
	if len(sealed) < n {
		return nil, errors.New("sealed value is truncated")
	}
	plain, err := b.aead.Open(nil, sealed[:n], sealed[n:], nil)
	if err != nil {
		return nil, errors.New("sealed value can't be opened: JOBS_SECRET_KEY changed or the data is corrupt")
	}
	return plain, nil
}

// Hint shows the last four characters of a key, for display.
func Hint(key string) string {
	if len(key) <= 8 {
		return "••••"
	}
	return "••••" + key[len(key)-4:]
}
