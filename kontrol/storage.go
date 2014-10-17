package kontrol

import (
	"errors"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/hashicorp/go-version"
	"github.com/koding/kite"
	"github.com/koding/kite/protocol"
)

var ErrKiteNotFound = errors.New("no kite is available")

// Storage is an interface to a kite storage.
type Storage interface {
	Get(query *protocol.KontrolQuery) (Kites, error)
	Set(key, value string) error
	Update(key, value string) error
	Delete(key string) error
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

func (e *Etcd) Delete(key string) error {
	_, err := e.client.Delete(key, true)
	return err
}

func (e *Etcd) Set(key, value string) error {
	_, err := e.client.Set(key, value, uint64(HeartbeatDelay/time.Second))
	return err
}

func (e *Etcd) Update(key, value string) error {
	_, err := e.client.Update(key, value, uint64(HeartbeatDelay/time.Second))
	return err
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
