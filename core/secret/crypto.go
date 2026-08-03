package secret

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
)

// DataKeyName is the SecretStore key holding the base64-encoded at-rest data key.
const DataKeyName = "omniproxy.atrest.key"

// ErrCorrupt indicates sealed data failed authentication (tampered or wrong key).
var ErrCorrupt = errors.New("secret: data corrupt or key mismatch")

// NewDataKey returns a fresh 32-byte AES-256 key.
func NewDataKey() ([]byte, error) {
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		return nil, fmt.Errorf("secret: generate data key: %w", err)
	}
	return k, nil
}

// GetOrCreateDataKey returns the stored data key, creating and persisting a new
// one when absent or invalid. name defaults to DataKeyName.
func GetOrCreateDataKey(store Store, name string) ([]byte, error) {
	if name == "" {
		name = DataKeyName
	}
	raw, err := store.Get(name)
	if err == nil {
		key, derr := base64.StdEncoding.DecodeString(raw)
		if derr == nil && len(key) == 32 {
			return key, nil
		}
		_ = store.Delete(name)
	}
	if !errors.Is(err, ErrNotFound) {
		return nil, fmt.Errorf("secret: read data key: %w", err)
	}

	key, kerr := NewDataKey()
	if kerr != nil {
		return nil, kerr
	}
	if serr := store.Set(name, base64.StdEncoding.EncodeToString(key)); serr != nil {
		return nil, fmt.Errorf("secret: persist data key: %w", serr)
	}
	return key, nil
}

// Seal encrypts plaintext with AES-256-GCM, returning nonce||ciphertext.
func Seal(key, plaintext []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, fmt.Errorf("secret: nonce: %w", err)
	}
	return gcm.Seal(nonce, nonce, plaintext, nil), nil
}

// Open authenticates and decrypts data produced by Seal.
func Open(key, data []byte) ([]byte, error) {
	gcm, err := newGCM(key)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(data) < ns {
		return nil, ErrCorrupt
	}
	plaintext, err := gcm.Open(nil, data[:ns], data[ns:], nil)
	if err != nil {
		return nil, ErrCorrupt
	}
	return plaintext, nil
}

func newGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("secret: cipher: %w", err)
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, fmt.Errorf("secret: gcm: %w", err)
	}
	return gcm, nil
}
