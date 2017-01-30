package kontrol

import (
	"math/rand"
	"strings"

	"github.com/hashicorp/go-version"
	"github.com/koding/kite/protocol"
)

// Kites is a helpe type to work with a set of kites
type Kites []*protocol.KiteWithToken

// Attach attaches the given token to each kite. It replaces any previous
// token.
func (k Kites) Attach(token string) {
	for _, kite := range k {
		kite.Token = token
	}
}

// Shuffle shuffles the order of the kites. This is useful if you want send
// back a randomized list of kites.
func (k *Kites) Shuffle() {
	shuffled := make(Kites, len(*k))
	for i, v := range rand.Perm(len(*k)) {
		shuffled[v] = (*k)[i]
	}

	*k = shuffled
}

// Filter filters out kites with the given constraints
func (k *Kites) Filter(constraint version.Constraints, keyRest string) {
	filtered := make(Kites, 0)
	for _, kite := range *k {
		if isValid(&kite.Kite, constraint, keyRest) {
			filtered = append(filtered, kite)
		}
	}

	*k = filtered
}

func isValid(k *protocol.Kite, c version.Constraints, keyRest string) bool {
	// Check the version constraint.
	v, _ := version.NewVersion(k.Version)
	if !c.Check(v) {
		return false
	}

	// Check the fields after version field.
	kiteKeyAfterVersion := "/" + strings.TrimRight(k.Region+"/"+k.Hostname+"/"+k.ID, "/")
	if !strings.HasPrefix(kiteKeyAfterVersion, keyRest) {
		return false
	}

	return true
}
