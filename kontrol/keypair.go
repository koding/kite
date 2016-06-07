package kontrol

import (
	"errors"
	"fmt"
	"time"

	"github.com/koding/cache"
)

// KeyPair defines a single key pair entity
type KeyPair struct {
	// ID is the unique id defining the key pair
	ID string

	// Public key is used to validate tokens
	Public string

	// Private key is used to sign/generate tokens
	Private string
}

func (k *KeyPair) Validate() error {
	if k.ID == "" {
		return errors.New("KeyPair ID field is empty")
	}

	if k.Public == "" {
		return errors.New("KeyPair Public field is empty")
	}

	if k.Private == "" {
		return errors.New("KeyPair Private field is empty")
	}

	return nil
}

// KeyPairStorage is responsible of managing key pairs
type KeyPairStorage interface {
	// AddKey adds the given key pair to the storage
	AddKey(*KeyPair) error

	// DeleteKey deletes the given key pairs from the storage
	DeleteKey(*KeyPair) error

	// GetKeyFromID retrieves the KeyPair from the given ID
	GetKeyFromID(id string) (*KeyPair, error)

	// GetKeyFromPublic retrieves the KeyPairs from the given public key.
	//
	// If the key is no longer valid and the storage is able to deterime
	// that it was deleted, the returned error is of *DeletedKeyPairError
	// type.
	GetKeyFromPublic(publicKey string) (*KeyPair, error)

	// Is valid checks if the given publicKey is valid or not. It's up to the
	// implementer how to implement it. A valid public key returns a nil error.
	//
	// If the key is no longer valid and the storage is able to deterime
	// that it was deleted, the returned error is of *DeletedKeyPairError
	// type.
	IsValid(publicKey string) error
}

func NewMemKeyPairStorage() *MemKeyPairStorage {
	return &MemKeyPairStorage{
		id:     cache.NewMemory(),
		public: cache.NewMemory(),
	}
}

func NewMemKeyPairStorageTTL(ttl time.Duration) *MemKeyPairStorage {
	return &MemKeyPairStorage{
		id:     cache.NewMemoryWithTTL(ttl),
		public: cache.NewMemoryWithTTL(ttl),
	}
}

type MemKeyPairStorage struct {
	id     cache.Cache
	public cache.Cache
}

func (m *MemKeyPairStorage) AddKey(keyPair *KeyPair) error {
	if err := keyPair.Validate(); err != nil {
		return err
	}

	m.id.Set(keyPair.ID, keyPair)
	m.public.Set(keyPair.Public, keyPair)
	return nil
}

func (m *MemKeyPairStorage) DeleteKey(keyPair *KeyPair) error {
	if keyPair.Public == "" {
		k, err := m.GetKeyFromID(keyPair.ID)
		if err != nil {
			return err
		}

		m.public.Delete(k.Public)
	}

	m.id.Delete(keyPair.ID)
	return nil
}

func (m *MemKeyPairStorage) GetKeyFromID(id string) (*KeyPair, error) {
	v, err := m.id.Get(id)
	if err != nil {
		return nil, err
	}

	keyPair, ok := v.(*KeyPair)
	if !ok {
		return nil, fmt.Errorf("MemKeyPairStorage: GetKeyFromID value is malformed %+v", v)
	}

	return keyPair, nil
}

func (m *MemKeyPairStorage) GetKeyFromPublic(public string) (*KeyPair, error) {
	v, err := m.public.Get(public)
	if err != nil {
		return nil, err
	}

	keyPair, ok := v.(*KeyPair)
	if !ok {
		return nil, fmt.Errorf("MemKeyPairStorage: GetKeyFromPublic value is malformed %+v", v)
	}

	return keyPair, nil
}

func (m *MemKeyPairStorage) IsValid(public string) error {
	_, err := m.GetKeyFromPublic(public)
	return err
}

// CachedStorage caches the requests that are going to backend and tries to
// lower the load on the backend
type CachedStorage struct {
	cache   KeyPairStorage
	backend KeyPairStorage
}

// NewCachedStorage creates a new CachedStorage
func NewCachedStorage(backend KeyPairStorage, cache KeyPairStorage) *CachedStorage {
	return &CachedStorage{
		cache:   cache,
		backend: backend,
	}
}

var _ KeyPairStorage = (&CachedStorage{})

func (m *CachedStorage) AddKey(keyPair *KeyPair) error {
	if err := m.backend.AddKey(keyPair); err != nil {
		return err
	}

	return m.cache.AddKey(keyPair)
}

func (m *CachedStorage) DeleteKey(keyPair *KeyPair) error {
	if err := m.backend.DeleteKey(keyPair); err != nil {
		return err
	}

	return m.cache.DeleteKey(keyPair)
}

func (m *CachedStorage) GetKeyFromID(id string) (*KeyPair, error) {
	if keyPair, err := m.cache.GetKeyFromID(id); err == nil {
		return keyPair, nil
	}

	keyPair, err := m.backend.GetKeyFromID(id)
	if err != nil {
		return nil, err
	}

	// set key to the cache
	if err := m.cache.AddKey(keyPair); err != nil {
		return nil, err
	}

	return keyPair, nil
}

func (m *CachedStorage) GetKeyFromPublic(public string) (*KeyPair, error) {
	if keyPair, err := m.cache.GetKeyFromPublic(public); err == nil {
		return keyPair, nil
	}

	keyPair, err := m.backend.GetKeyFromPublic(public)
	if err != nil {
		return nil, err
	}

	// set key to the cache
	if err := m.cache.AddKey(keyPair); err != nil {
		return nil, err
	}

	return keyPair, nil
}

func (m *CachedStorage) IsValid(public string) error {
	if err := m.cache.IsValid(public); err == nil {
		return nil
	}

	return m.backend.IsValid(public)
}
