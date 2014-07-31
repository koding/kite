package kontrol

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/coreos/etcd/store"
	"github.com/koding/kite/protocol"
)

var keyOrder = []string{"username", "environment", "name", "version", "region", "hostname", "id"}

// validateKiteKey returns a string representing the kite uniquely
// that is suitable to use as a key for etcd.
func validateKiteKey(k *protocol.Kite) error {
	fields := k.Query().Fields()

	// Validate fields.
	for k, v := range fields {
		if v == "" {
			return fmt.Errorf("Empty Kite field: %s", k)
		}
		if strings.ContainsRune(v, '/') {
			return fmt.Errorf("Field \"%s\" must not contain '/'", k)
		}
	}

	return nil
}

func (k *Kontrol) etcdKeyFromId(id string) (string, error) {
	log.Info("Searching for %s", KitesPrefix+"/"+id)

	event, err := k.etcd.Store.Get(
		KitesPrefix+"/"+id, // path
		false, // recursive, return all child directories too
		false, // sorting flag, we don't care about sorting for now
	)

	if err != nil {
		if err2, ok := err.(*etcdErr.Error); ok && err2.ErrorCode == etcdErr.EcodeKeyNotFound {
			return "", nil
		}

		log.Error("etcd error: %s", err)
		return "", fmt.Errorf("internal error - getKites")
	}

	fmt.Printf("event %+v\n", event)
	fmt.Printf("event node %+v\n", event.Node)

	return *event.Node.Value, nil
}

// onlyIDQuery returns true if the query contains only a non-empty ID and all
// others keys are empty
func onlyIDQuery(q *protocol.KontrolQuery) bool {
	fields := q.Fields()

	// check if any other key exist, if yes return a false
	for _, k := range keyOrder {
		v := fields[k]
		if k != "id" && v != "" {
			return false
		}
	}

	// now all other keys are empty, check finally for our ID
	if fields["id"] != "" {
		return true
	}

	// ID is empty too!
	return false
}

// getQueryKey returns the etcd key for the query.
func (k *Kontrol) getQueryKey(q *protocol.KontrolQuery) (string, error) {
	// check first if it's an ID search
	a := q.Fields()
	fmt.Printf("a %+v\n", a)
	if onlyIDQuery(q) {
		return k.etcdKeyFromId(q.ID)
	}

	fields := q.Fields()

	if q.Username == "" {
		return "", errors.New("Empty username field")
	}

	// Validate query and build key.
	path := "/"

	empty := false   // encountered with empty field?
	empytField := "" // for error log

	// http://golang.org/doc/go1.3#map, order is important and we can't rely on
	// maps because the keys are not ordered :)
	for _, key := range keyOrder {
		v := fields[key]
		if v == "" {
			empty = true
			empytField = key
			continue
		}

		if empty && v != "" {
			return "", fmt.Errorf("Invalid query. Query option is not set: %s", empytField)
		}

		path = path + v + "/"
	}

	path = strings.TrimSuffix(path, "/")

	fmt.Printf("returning path %+v\n", path)

	return path, nil
}

func getAudience(q protocol.KontrolQuery) string {
	if q.Name != "" {
		return "/" + q.Username + "/" + q.Environment + "/" + q.Name
	} else if q.Environment != "" {
		return "/" + q.Username + "/" + q.Environment
	} else {
		return "/" + q.Username
	}
}

// flatten converts the recursive etcd directory structure to flat one that contains Kites.
func flatten(in store.NodeExterns) (out store.NodeExterns) {
	for _, node := range in {
		if node.Dir {
			out = append(out, flatten(node.Nodes)...)
			continue
		}

		out = append(out, node)
	}

	return
}

func kitesFromNodes(nodes store.NodeExterns) ([]*protocol.KiteWithToken, error) {
	kites := make([]*protocol.KiteWithToken, len(nodes))

	for i, node := range nodes {
		var rv registerValue
		json.Unmarshal([]byte(*node.Value), &rv)

		kite, _ := kiteFromEtcdKey(node.Key)

		kites[i] = &protocol.KiteWithToken{
			Kite: *kite,
			URL:  rv.URL,
		}
	}

	return kites, nil
}

func kiteWithTokenFromEtcdNode(node *store.NodeExtern, token string) (*protocol.KiteWithToken, error) {
	kite, err := kiteFromEtcdKey(node.Key)
	if err != nil {
		return nil, err
	}

	var rv registerValue
	err = json.Unmarshal([]byte(*node.Value), &rv)
	if err != nil {
		return nil, err
	}

	return &protocol.KiteWithToken{
		Kite:  *kite,
		URL:   rv.URL,
		Token: token,
	}, nil
}

// kiteFromEtcdKey returns a *protocol.Kite from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKey(key string) (*protocol.Kite, error) {
	// TODO replace "kites" with KitesPrefix constant
	fields := strings.Split(strings.TrimPrefix(key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, fmt.Errorf("Invalid Kite: %s", key)
	}

	return &protocol.Kite{
		Username:    fields[1],
		Environment: fields[2],
		Name:        fields[3],
		Version:     fields[4],
		Region:      fields[5],
		Hostname:    fields[6],
		ID:          fields[7],
	}, nil
}
