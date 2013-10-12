package main

import (
	"github.com/fatih/goset"
	"sync"
)

type KiteDependency struct {
	r map[string]*goset.Set
	sync.Mutex
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

	k.Lock()
	defer k.Unlock()

	if k.r[source] == nil {
		k.r[source] = goset.New()
	}

	k.r[source].Add(target)
}

func (k *KiteDependency) Remove(source string) {
	if source == "" {
		return
	}

	k.Lock()
	defer k.Unlock()

	if k.r[source] == nil {
		return
	}

	k.r[source].Clear()
}

func (k *KiteDependency) Has(source string) bool {
	if source == "" {
		return false
	}
	k.Lock()
	defer k.Unlock()

	s, ok := k.r[source]
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
	k.Lock()
	defer k.Unlock()

	s, ok := k.r[source]
	if !ok {
		return make([]string, 0)
	}

	return s.StringSlice()
}
