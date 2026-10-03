package secrets

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/zalando/go-keyring"
)

// Service is the OS keychain service name for every stored secret.
const Service = "agent-office"

const refPrefix = "secret://"

var (
	ErrNotFound   = errors.New("secret not found")
	ErrInvalidRef = errors.New("invalid secret ref")
)

// Store keeps secret values out of the database. Only the ref returned by
// NewRef is ever persisted elsewhere.
type Store interface {
	Set(ref, value string) error
	Get(ref string) (string, error)
	Delete(ref string) error
	// Persistent is false when values only live in memory for this session.
	Persistent() bool
}

// NewRef returns a fresh, unguessable reference for a new secret.
func NewRef() string {
	b := make([]byte, 16)
	rand.Read(b)
	return refPrefix + hex.EncodeToString(b)
}

func account(ref string) (string, error) {
	id, ok := strings.CutPrefix(ref, refPrefix)
	if !ok || id == "" {
		return "", fmt.Errorf("%w: %q", ErrInvalidRef, ref)
	}
	return id, nil
}

// Open returns the OS keychain store when it works on this machine,
// otherwise an in-memory store. The probe writes and removes a throwaway
// entry so a broken keychain is detected before any real key is saved.
func Open() Store {
	ks := keyringStore{}
	probe := NewRef()
	if err := ks.Set(probe, "probe"); err == nil {
		v, err := ks.Get(probe)
		ks.Delete(probe)
		if err == nil && v == "probe" {
			return ks
		}
	}
	return NewMemory()
}

type keyringStore struct{}

func (keyringStore) Persistent() bool { return true }

func (keyringStore) Set(ref, value string) error {
	acct, err := account(ref)
	if err != nil {
		return err
	}
	return keyring.Set(Service, acct, value)
}

func (keyringStore) Get(ref string) (string, error) {
	acct, err := account(ref)
	if err != nil {
		return "", err
	}
	v, err := keyring.Get(Service, acct)
	if errors.Is(err, keyring.ErrNotFound) {
		return "", ErrNotFound
	}
	return v, err
}

func (keyringStore) Delete(ref string) error {
	acct, err := account(ref)
	if err != nil {
		return err
	}
	if err := keyring.Delete(Service, acct); err != nil && !errors.Is(err, keyring.ErrNotFound) {
		return err
	}
	return nil
}

type memoryStore struct {
	mu   sync.Mutex
	vals map[string]string
}

// NewMemory returns a store that forgets everything when the app exits.
func NewMemory() Store { return &memoryStore{vals: map[string]string{}} }

func (*memoryStore) Persistent() bool { return false }

func (m *memoryStore) Set(ref, value string) error {
	if _, err := account(ref); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	m.vals[ref] = value
	return nil
}

func (m *memoryStore) Get(ref string) (string, error) {
	if _, err := account(ref); err != nil {
		return "", err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.vals[ref]
	if !ok {
		return "", ErrNotFound
	}
	return v, nil
}

func (m *memoryStore) Delete(ref string) error {
	if _, err := account(ref); err != nil {
		return err
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.vals, ref)
	return nil
}
