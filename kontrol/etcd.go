package kontrol

import (
	"errors"
	"fmt"
	"strings"

	etcdErr "github.com/coreos/etcd/error"
	"github.com/koding/kite/protocol"
)

// keyOrder defines the order of the query paramaters.
var keyOrder = []string{
	"username",
	"environment",
	"name",
	"version",
	"region",
	"hostname",
	"id",
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	URL string `json:"url"`
}

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

// etcdKeyFromId returns the value for a single full ID path
func (k *Kontrol) etcdKeyFromId(id string) (string, error) {
	log.Info("Searching etcd key from id %s", KitesPrefix+"/"+id)

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
