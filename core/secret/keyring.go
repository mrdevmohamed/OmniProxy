package secret

import (
	"errors"

	"github.com/zalando/go-keyring"
)

// Keyring wraps go-keyring: Linux Secret Service (libsecret), Windows
// Credential Manager/DPAPI. The service name scopes all entries.
type Keyring struct {
	service string
}

// NewKeyring returns a Keyring for the given service name (default "omniproxy").
func NewKeyring(service string) *Keyring {
	if service == "" {
		service = "omniproxy"
	}
	return &Keyring{service: service}
}

// Get implements Store.
func (k *Keyring) Get(key string) (string, error) {
	v, err := keyring.Get(k.service, key)
	if err != nil {
		if errors.Is(err, keyring.ErrNotFound) {
			return "", ErrNotFound
		}
		return "", err
	}
	return v, nil
}

// Set implements Store.
func (k *Keyring) Set(key, value string) error {
	return keyring.Set(k.service, key, value)
}

// Delete implements Store.
func (k *Keyring) Delete(key string) error {
	return keyring.Delete(k.service, key)
}
