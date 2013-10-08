package kite

import "sync"

type Balancer struct {
	i map[string]int
	sync.RWMutex
}

func NewBalancer() *Balancer {
	return &Balancer{
		i: make(map[string]int),
	}
}

// used for loadbalance modes, like roundrobin or random
// getIndex is used to get the current index for current the loadbalance
// algorithm/mode. It's concurrent-safe.
func (b *Balancer) GetIndex(kite string) int {
	b.RLock()
	defer b.RUnlock()
	index, _ := b.i[kite]
	return index
}

// addOrUpdateIndex is used to add the current index for the current loadbalacne
// algorithm. The index number is changed according to to the loadbalance mode.
// When used roundrobin, the next items index is saved, for random a random
// number is assigned, and so on. It's concurrent-safe.
func (b *Balancer) AddOrUpdateIndex(kite string, index int) {
	b.Lock()
	defer b.Unlock()
	b.i[kite] = index
}

// deleteIndex is used to remove the current index from the indexes. It's
// concurrent-safe.
func (b *Balancer) DeleteIndex(host string) {
	b.Lock()
	defer b.Unlock()
	delete(b.i, host)
}
