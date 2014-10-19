package kontrol

import (
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/hashicorp/go-version"
	"github.com/koding/kite"

	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

var ErrKiteNotFound = errors.New("no kite is available")

// Storage is an interface to a kite storage.
type Storage interface {
	// Get retrieves the Kites with the given query
	Get(query *protocol.KontrolQuery) (Kites, error)

	// Set stores the given kite with the given value
	Set(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error

	// Delete deletes the given kite from the storage
	Delete(kite *protocol.Kite) error
}

// Etcd implements the Storage interface
type Etcd struct {
	client *etcd.Client
	log    kite.Logger
}

func NewEtcd(machines []string) (*Etcd, error) {
	if machines == nil || len(machines) == 0 {
		return nil, errors.New("machines is empty")
	}

	client := etcd.NewClient(machines)
	ok := client.SetCluster(machines)
	if !ok {
		return nil, errors.New("cannot connect to etcd cluster: " + strings.Join(machines, ","))
	}

	return &Etcd{
		client: client,
	}, nil
}

func (e *Etcd) Delete(k *protocol.Kite) error {
	etcdKey := KitesPrefix + k.String()
	etcdIDKey := KitesPrefix + "/" + k.ID

	_, err := e.client.Delete(etcdKey, true)
	_, err = e.client.Delete(etcdIDKey, true)
	return err
}

func (e *Etcd) Set(k *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	etcdKey := KitesPrefix + k.String()
	etcdIDKey := KitesPrefix + "/" + k.ID

	valueBytes, _ := json.Marshal(value)
	valueString := string(valueBytes)

	// Set the kite key.
	// Example "/koding/production/os/0.0.1/sj/kontainer1.sj.koding.com/1234asdf..."
	_, err := e.client.Set(etcdKey, valueString, uint64(HeartbeatDelay/time.Second))
	if err != nil {
		return err
	}

	// Also store the the kite.Key Id for easy lookup
	_, err = e.client.Set(etcdIDKey, valueString, uint64(HeartbeatDelay/time.Second))
	if err != nil {
		return err
	}

	// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
	_, err = e.client.Update(KitesPrefix+"/"+k.Username,
		"", uint64(HeartbeatDelay/time.Second))
	if err != nil {
		return err
	}

	return nil
}

func (e *Etcd) Get(query *protocol.KontrolQuery) (Kites, error) {
	// We will make a get request to etcd store with this key. So get a "etcd"
	// key from the given query so that we can use it to query from Etcd.
	etcdKey, err := e.etcdKey(query)
	if err != nil {
		return nil, err
	}

	// If version field contains a constraint we need no make a new query up to
	// "name" field and filter the results after getting all versions.
	// NewVersion returns an error if it's a constraint, like: ">= 1.0, < 1.4"
	// Because NewConstraint doesn't return an error for version's like "0.0.1"
	// we check it with the NewVersion function.
	var hasVersionConstraint bool // does query contains a constraint on version?
	var keyRest string            // query key after the version field
	var versionConstraint version.Constraints
	_, err = version.NewVersion(query.Version)
	if err != nil && query.Version != "" {
		// now parse our constraint
		versionConstraint, err = version.NewConstraint(query.Version)
		if err != nil {
			// version is a malformed, just return the error
			return nil, err
		}

		hasVersionConstraint = true
		nameQuery := &protocol.KontrolQuery{
			Username:    query.Username,
			Environment: query.Environment,
			Name:        query.Name,
		}
		// We will make a get request to all nodes under this name
		// and filter the result later.
		etcdKey, _ = GetQueryKey(nameQuery)

		// Rest of the key after version field
		keyRest = "/" + strings.TrimRight(
			query.Region+"/"+query.Hostname+"/"+query.ID, "/")
	}

	resp, err := e.client.Get(KitesPrefix+etcdKey, false, true)
	if err != nil {
		// if it's something else just return
		return nil, err
	}

	kites := make(Kites, 0)

	node := NewNode(resp.Node)

	// means a query with all fields were made or a query with an ID was made,
	// in which case also returns a full path. This path has a value that
	// contains the final kite URL. Therefore this is a single kite result,
	// create it and pass it back.
	if node.HasValue() {
		oneKite, err := node.Kite()
		if err != nil {
			return nil, err
		}

		kites = append(kites, oneKite)
	} else {
		kites, err = node.Kites()
		if err != nil {
			return nil, err
		}

		// Filter kites by version constraint
		if hasVersionConstraint {
			kites.Filter(versionConstraint, keyRest)
		}
	}

	// Shuffle the list
	kites.Shuffle()

	return kites, nil
}

func (e *Etcd) etcdKey(query *protocol.KontrolQuery) (string, error) {
	if onlyIDQuery(query) {
		resp, err := e.client.Get(KitesPrefix+"/"+query.ID, false, true)
		if err != nil {
			return "", err
		}

		return resp.Node.Value, nil
	}

	return GetQueryKey(query)
}
