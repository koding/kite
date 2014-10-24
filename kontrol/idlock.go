package kontrol

import "sync"

type IdLock struct {
	locks   map[string]sync.Locker
	locksMu sync.Mutex
}

// New returns a new IdLock
func NewIdlock() *IdLock {
	return &IdLock{
		locks: make(map[string]sync.Locker),
	}

}

// Get returns a lock that is bound to a specific id.
func (i *IdLock) Get(id string) sync.Locker {
	i.locksMu.Lock()
	defer i.locksMu.Unlock()

	var l sync.Locker
	var ok bool

	l, ok = i.locks[id]
	if !ok {
		l = &sync.Mutex{}
		i.locks[id] = l
	}

	return l
}
