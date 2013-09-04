package main

import (
	"github.com/fatih/goset"
	"sync"
)

type KiteDependency struct {
	r map[string]*goset.Set
	sync.RWMutex
}

func NewDependency() *KiteDependency {
	return &KiteDependency{
		r: make(map[string]*goset.Set),
	}
}

// Add relationsips to kite
func (k *KiteDependency) Add(source, target string) {
	if target == "" || source == "" {
		return
	}

	k.RLock()
	s := k.r[source]
	k.RUnlock()

	if s == nil {
		s = goset.New()
	}

	k.Lock()
	s.Add(target)
	k.r[source] = s
	k.Unlock()
}

func (k *KiteDependency) Remove(source string) {
	if source == "" {
		return
	}

	k.RLock()
	s := k.r[source]
	k.RUnlock()

	if s == nil {
		s = goset.New()
	}

	k.Lock()
	s.Clear()
	k.r[source] = s
	k.Unlock()
}

func (k *KiteDependency) Has(source string) bool {
	if source == "" {
		return false
	}

	k.RLock()
	s, ok := k.r[source]
	k.RUnlock()
	if !ok {
		return false
	}

	if s == nil {
		return false
	}

	return true
}

// ListRelationship returns a slice of kite names that depends on "source"
// It returns an empty slice if the kite doesn't have any relationships.
func (k *KiteDependency) List(source string) []string {
	k.RLock()
	defer k.RUnlock()
	s, ok := k.r[source]
	if !ok {
		return make([]string, 0)
	}

	return s.StringSlice()
}
