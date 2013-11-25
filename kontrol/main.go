package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coreos/go-etcd/etcd"
	logging "github.com/op/go-logging"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/dnode"
	"koding/newkite/kite"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"koding/tools/config"
	stdlog "log"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	HEARTBEAT_INTERVAL = time.Millisecond * 1000
	HEARTBEAT_DELAY    = time.Millisecond * 2000
)

var log = logging.MustGetLogger("Kontrol")

type Kontrol struct {
	etcd *etcd.Client
}

func NewKontrol() *Kontrol {
	return &Kontrol{
		etcd: etcd.NewClient(nil), // TODO read machine list from config
	}
}

func main() {
	setupLogging()

	kontrol := NewKontrol()

	options := &protocol.Options{
		Kitename:    "kontrol",
		Version:     "1",
		Port:        strconv.Itoa(config.Current.NewKontrol.Port),
		Region:      "localhost",
		Environment: "development",
	}

	k := kite.New(options)
	k.KontrolEnabled = false

	k.Authenticators["kodingKey"] = kontrol.AuthenticateFromKodingKey
	k.Authenticators["sessionID"] = kontrol.AuthenticateFromSessionID

	k.HandleFunc("register", kontrol.handleRegister)
	k.HandleFunc("getKites", kontrol.handleGetKites)
	// k.HandleFunc("watchKites", kontrol.handleWatchKites)

	k.Run()
}

func setupLogging() {
	log.Module = "Kontrol"
	logging.SetFormatter(logging.MustStringFormatter("%{level:-8s} ▶ %{message}"))
	stderrBackend := logging.NewLogBackend(os.Stderr, "", stdlog.LstdFlags|stdlog.Lshortfile)
	stderrBackend.Color = true
	syslogBackend, _ := logging.NewSyslogBackend(log.Module)
	logging.SetBackend(stderrBackend, syslogBackend)
}

// registerValue is the type of the value that is saved to etcd.
type registerValue struct {
	PublicIP  string
	Port      string
	KodingKey string
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %#v", r.RemoteKite.Kite)

	// Only accept requests with kodingKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kodingKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	// Set PublicIP address if it's empty.
	if r.RemoteKite.PublicIP == "" {
		r.RemoteKite.PublicIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	key, err := getKiteKey(r.RemoteKite.Kite)
	if err != nil {
		return nil, err
	}

	rv := &registerValue{
		PublicIP:  r.RemoteKite.PublicIP,
		Port:      r.RemoteKite.Port,
		KodingKey: r.Authentication.Key,
	}

	valueBytes, _ := json.Marshal(rv)
	value := string(valueBytes)

	ttl := uint64(HEARTBEAT_DELAY / time.Second)

	// setKey sets the value of the Kite in etcd.
	setKey := func() (prevValue string, err error) {
		resp, err := k.etcd.Set(key, value, ttl)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return
		}

		prevValue = resp.PrevValue

		// Set the TTL for the username. Otherwise, empty dirs remain in etcd.
		_, err = k.etcd.UpdateDir("/"+r.RemoteKite.Username, ttl)
		if err != nil {
			log.Critical("etcd error: %s", err)
			return
		}

		return
	}

	// Register to etcd.
	prev, err := setKey()
	if err != nil {
		return nil, errors.New("Internal error")
	}

	if prev != "" {
		log.Notice("Kite (%s) is already registered. Doing nothing.", key)
	} else {
		// Request heartbeat from the Kite.

		heartbeatFunc := func(p *dnode.Partial) {
			prev, err := setKey()
			if err == nil && prev == "" {
				log.Warning("Came heartbeat but the Kite (%s) is not registered. Re-registering it. It may be an indication that the heartbeat delay is too short.", key)
			}
		}

		heartbeatArgs := []interface{}{
			HEARTBEAT_INTERVAL / time.Second,
			dnode.Callback(heartbeatFunc),
		}

		_, err := r.RemoteKite.Call("heartbeat", heartbeatArgs)
		if err != nil {
			return nil, err
		}
	}

	log.Info("Kite registered: %s", key)

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResult{
		Result:   protocol.AllowKite,
		Username: r.RemoteKite.Username,
		PublicIP: r.RemoteKite.PublicIP,
	}

	return response, nil
}

func getKiteKey(k protocol.Kite) (string, error) {
	// Order is important.
	fields := map[string]string{
		"username":    k.Username,
		"environment": k.Environment,
		"name":        k.Name,
		"version":     k.Version,
		"region":      k.Region,
		"hostname":    k.Hostname,
		"id":          k.ID,
	}

	// Validate fields.
	for k, v := range fields {
		if v == "" {
			return "", fmt.Errorf("Empty Kite field: %s", k)
		}
		if strings.ContainsRune(v, '/') {
			return "", fmt.Errorf("Field \"%s\" must not contain '/'", k)
		}
	}

	// Build key.
	key := "/"
	for _, v := range fields {
		key = key + v + "/"
	}
	key = strings.TrimSuffix(key, "/")

	return key, nil
}

func getQueryKey(q *KontrolQuery) (string, error) {
	fields := []string{
		q.Username,
		q.Environment,
		q.Name,
		q.Version,
		q.Region,
		q.Hostname,
		q.ID,
	}

	// Validate query and build key.
	path := "/"
	empty := false
	for _, f := range fields {
		if f == "" {
			empty = true
		} else {
			if empty {
				return "", errors.New("Invalid query")
			}
			path = path + f + "/"
		}
	}

	return path, nil
}

// 	// Deregister the Kite on disconnect.
// 	r.RemoteKite.OnDisconnect(func() {
// 		log.Info("Deregistering Kite: %s", r.RemoteKite.ID)
// 		k.storage.Remove(r.RemoteKite.ID)

// 		// Delete from jVMs when all kites to that user is died.
// 		if !k.kitesExistsForUser(r.RemoteKite.Username) {
// 			deleteFromVM(r.RemoteKite.Username)
// 		}
// 	})

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	query := new(KontrolQuery)
	err := r.Args.Unmarshal(query)
	if err != nil {
		return nil, err
	}

	// We do not allow access to other's kites for now.
	if r.Username != query.Username {
		return nil, errors.New("Not your Kite")
	}

	key, err := getQueryKey(query)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.GetAll(key, false)
	if err != nil {
		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("Internal error")
	}

	kvs := flatten(resp.Kvs)

	kitesWithToken, err := addTokenToKites(kvs, r.Username)
	if err != nil {
		return nil, err
	}

	return kitesWithToken, nil
}

// flatten converts the recursive etcd directory structure to flat one that contains Kites.
func flatten(in []etcd.KeyValuePair) []etcd.KeyValuePair {
	var out []etcd.KeyValuePair

	for _, kv := range in {
		if kv.Dir {
			out = append(out, flatten(kv.KVPairs)...)
			continue
		}

		out = append(out, kv)
	}

	return out
}

func addTokenToKites(kvs []etcd.KeyValuePair, username string) ([]protocol.KiteWithToken, error) {
	kitesWithToken := make([]protocol.KiteWithToken, len(kvs))

	for i, kv := range kvs {
		kite, kodingKey, err := kiteFromEtcdKV(&kv)
		if err != nil {
			return nil, err
		}

		// Generate token.
		key, err := kodingkey.FromString(kodingKey)
		if err != nil {
			return nil, fmt.Errorf("Koding Key is invalid at Kite: %s", key)
		}

		// username is from requester, key is from kite owner.
		tokenString, err := token.NewToken(username, kite.ID).EncryptString(key)
		if err != nil {
			return nil, errors.New("Server error: Cannot generate a token")
		}

		kitesWithToken[i] = protocol.KiteWithToken{
			Kite:  *kite,
			Token: tokenString,
		}
	}

	return kitesWithToken, nil
}

func kiteFromEtcdKV(kv *etcd.KeyValuePair) (*protocol.Kite, string, error) {
	key := strings.TrimPrefix(kv.Key, "/")

	fields := strings.Split(key, "/")
	if len(fields) != 7 {
		log.Critical("Key does not represent a Kite: %s", kv.Key)
		return nil, "", errors.New("Internal error")
	}

	kite := new(protocol.Kite)
	kite.Username = fields[0]
	kite.Environment = fields[1]
	kite.Name = fields[2]
	kite.Version = fields[3]
	kite.Region = fields[4]
	kite.Hostname = fields[5]
	kite.ID = fields[6]

	value := new(registerValue)
	json.Unmarshal([]byte(kv.Value), value)

	kite.PublicIP = value.PublicIP
	kite.Port = value.Port

	return kite, value.KodingKey, nil
}

// func (k *Kontrol) handleWatchKites(r *kite.Request) (interface{}, error) {
// 	var args []*dnode.Partial
// 	err := r.Args.Unmarshal(&args)
// 	if err != nil {
// 		return nil, err
// 	}

// 	if len(args) != 2 {
// 		return nil, errors.New("Invalid number of arguments")
// 	}

// 	var query KontrolQuery
// 	err = args[0].Unmarshal(&query)
// 	if err != nil {
// 		return nil, err
// 	}

// 	var callback dnode.Function
// 	err = args[1].Unmarshal(&callback)
// 	if err != nil {
// 		return nil, err
// 	}

// 	// We do not allow access to other's kites for now.
// 	if r.Username != query.Username {
// 		return nil, errors.New("Not your Kite")
// 	}

// 	k.watchers.RegisterWatcher(r.RemoteKite, &query, callback)
// 	return nil, nil
// }

// func deleteFromVM(username string) error {
// 	if username == "" {
// 		return errors.New("deleting local vm err: empty username is passed")
// 	}

// 	hostnameAlias := "local-" + username
// 	err := modelhelper.DeleteVM(hostnameAlias)
// 	if err != nil {
// 		return fmt.Errorf("deleting local vm err:", err)
// 	}
// 	return nil
// }

// func (k *Kontrol) kitesExistsForUser(username string) bool {
// 	found := false

// 	for _, kite := range k.storage.List() {
// 		if kite.Username == username {
// 			found = true
// 		}
// 	}

// 	return found
// }

func (k *Kontrol) AuthenticateFromSessionID(options *kite.CallOptions) error {
	username, err := findUsernameFromSessionID(options.Authentication.Key)
	if err != nil {
		return err
	}

	options.Kite.Username = username

	return nil
}

func findUsernameFromSessionID(sessionID string) (string, error) {
	session, err := modelhelper.GetSession(sessionID)
	if err != nil {
		return "", err
	}

	return session.Username, nil
}

func (k *Kontrol) AuthenticateFromKodingKey(options *kite.CallOptions) error {
	username, err := findUsernameFromKey(options.Authentication.Key)
	if err != nil {
		return err
	}

	options.Kite.Username = username

	return nil
}

func findUsernameFromKey(key string) (string, error) {
	kodingKey, err := modelhelper.GetKodingKeysByKey(key)
	if err != nil {
		return "", fmt.Errorf("register kodingkey err %s", err)
	}

	account, err := modelhelper.GetAccountById(kodingKey.Owner)
	if err != nil {
		return "", fmt.Errorf("register get user err %s", err)
	}

	if account.Profile.Nickname == "" {
		return "", errors.New("nickname is empty, could not register kite")
	}

	return account.Profile.Nickname, nil
}
