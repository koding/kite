package peers

import (
	"koding/db/models"
	"sync"
)

// Kites is a concurrent safe abstraction package that let us add, remove, get
// , list data in form of models.Kite
type Kites struct {
	m map[string]*models.Kite
	sync.Mutex
}

func New() *Kites {
	return &Kites{
		m: make(map[string]*models.Kite),
	}
}

// Add registers or replaces a new models.Kite to the global map
func (k *Kites) Add(kite *models.Kite) {
	if kite == nil {
		return
	}

	k.Lock()
	defer k.Unlock()

	k.m[kite.Uuid] = kite
}

// Get returns the specified kite via its Uuid.
func (k *Kites) Get(id string) *models.Kite {
	k.Lock()
	defer k.Unlock()

	kite, ok := k.m[id]
	if !ok {
		return nil
	}

	return kite
}

// Remove deletes the specified kite from the registry.
func (k *Kites) Remove(id string) {
	k.Lock()
	defer k.Unlock()

	delete(k.m, id)
}

// Has looks for the existence of a kite. If an Uuid already exists in the
// registry, it returns true.
func (k *Kites) Has(id string) bool {
	k.Lock()
	defer k.Unlock()

	_, ok := k.m[id]
	return ok
}

// Has looks for the existence of a kite. If an Uuid already exists in the
// registry, it returns true.
func (k *Kites) Size() int {
	k.Lock()
	defer k.Unlock()

	return len(k.m)
}

// List returns a slice of all active kites.
func (k *Kites) List() []*models.Kite {
	k.Lock()
	defer k.Unlock()

	kites := make([]*models.Kite, 0)
	for _, kite := range k.m {
		kites = append(kites, kite)
	}
	return kites
}
