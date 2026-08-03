// Package secret provides OS-native secure storage (PRD §9): the Store
// interface implemented per platform (Android Keystore via native glue, Windows
// Credential Manager/DPAPI and Linux Secret Service via go-keyring), plus the
// at-rest encryption primitives that seal persisted configuration.
package secret

import (
	"errors"
	"sync"
)

// ErrNotFound is returned by Store.Get for absent keys.
var ErrNotFound = errors.New("secret: not found")

// Store is the OS-native secure storage boundary. Implementations must be
// safe for concurrent use.
type Store interface {
	// Get returns the value for key, or ErrNotFound.
	Get(key string) (string, error)
	// Set stores value for key, replacing any previous value.
	Set(key, value string) error
	// Delete removes key. Deleting a missing key is not an error.
	Delete(key string) error
}

// InMemory is a Store backed by a map. Intended for tests and as a fallback
// where no OS secure storage is available.
type InMemory struct {
	mu sync.RWMutex
	m  map[string]string
}

// NewInMemory returns an empty InMemory store.
func NewInMemory() *InMemory {
	return &InMemory{m: make(map[string]string)}
}

// Get implements Store.
func (s *InMemory) Get(key string) (string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.m[key]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

// Set implements Store.
func (s *InMemory) Set(key, value string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = value
	return nil
}

// Delete implements Store.
func (s *InMemory) Delete(key string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
	return nil
}

// Keys returns all stored keys (test/debug helper).
func (s *InMemory) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]string, 0, len(s.m))
	for k := range s.m {
		out = append(out, k)
	}
	return out
}

// Len returns the number of stored entries.
func (s *InMemory) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.m)
}
