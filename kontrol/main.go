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
	HEARTBEAT_INTERVAL = 1 * time.Minute
	HEARTBEAT_DELAY    = 2 * time.Minute
)

var log = logging.MustGetLogger("Kontrol")

type Kontrol struct {
	etcd       *etcd.Client
	watcherHub *watcherHub
}

func NewKontrol() *Kontrol {
	// Read list of etcd servers from config.
	machines := make([]string, len(config.Current.Etcd))
	for i, s := range config.Current.Etcd {
		machines[i] = "http://" + s.Host + ":" + strconv.FormatUint(uint64(s.Port), 10)
	}

	return &Kontrol{
		etcd:       etcd.NewClient(machines),
		watcherHub: newWatcherHub(),
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

	go kontrol.WatchEtcd()

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
		_, err = k.etcd.UpdateDir("/kites/"+r.RemoteKite.Username, ttl)
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
	k.watcherHub.Notify(&r.RemoteKite.Kite, protocol.Register)

	r.RemoteKite.OnDisconnect(func() {
		// Delete from etcd, WatchEtcd() will get the event
		// and will notify watchers of this Kite for deregistration.
		k.etcd.Delete(key)
	})

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResult{
		Result:   protocol.AllowKite,
		Username: r.RemoteKite.Username,
		PublicIP: r.RemoteKite.PublicIP,
	}

	return response, nil
}

// getKiteKey returns a string representing the kite uniquely
// that is suitable to use as a key for etcd.
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

	return "/kites" + key, nil
}

// getQueryKey returns the etcd key for the query.
func getQueryKey(q *protocol.KontrolQuery) (string, error) {
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

	return "/kites" + path, nil
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	var args []*dnode.Partial
	err := r.Args.Unmarshal(&args)
	if err != nil {
		return nil, err
	}

	if len(args) != 1 && len(args) != 2 {
		return nil, errors.New("Invalid number of arguments")
	}

	var query protocol.KontrolQuery
	err = args[0].Unmarshal(&query)
	if err != nil {
		return nil, errors.New("Invalid query argument")
	}

	// To be called when a Kite is registered or deregistered matching the query.
	var watchCallback dnode.Function
	if len(args) == 2 {
		err = args[1].Unmarshal(&watchCallback)
		if err != nil {
			return nil, errors.New("Invalid callback argument")
		}
	}

	// We do not allow access to other's kites for now.
	if r.Username != query.Username {
		return nil, errors.New("Not your Kite")
	}

	return k.getKites(r, query, watchCallback)
}

func (k *Kontrol) getKites(r *kite.Request, query protocol.KontrolQuery, watchCallback dnode.Function) ([]protocol.KiteWithToken, error) {
	key, err := getQueryKey(&query)
	if err != nil {
		return nil, err
	}

	resp, err := k.etcd.GetAll(key, false)
	if err != nil {
		if etcdErr, ok := err.(etcd.EtcdError); ok {
			if etcdErr.ErrorCode == 100 { // Key Not Found
				return make([]protocol.KiteWithToken, 0), nil
			}
		}
		log.Critical("etcd error: %s", err)
		return nil, fmt.Errorf("Internal error")
	}

	kvs := flatten(resp.Kvs)

	kitesWithToken, err := addTokenToKites(kvs, r.Username)
	if err != nil {
		return nil, err
	}

	// Register callbacks to our watcher hub.
	// It will call them when a Kite registered/unregistered matching the query.
	if watchCallback != nil {
		k.watcherHub.RegisterWatcher(r.RemoteKite, &query, watchCallback)
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
		kite, kodingKey, err := kiteFromEtcdKV(kv.Key, kv.Value)
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

// kiteFromEtcdKV returns a *protocol.Kite and Koding Key string from an etcd key.
// etcd key is like: /kites/devrim/development/mathworker/1/localhost/tardis.local/662ed473-351f-4c9f-786b-99cf02cdaadb
func kiteFromEtcdKV(key, value string) (*protocol.Kite, string, error) {
	fields := strings.Split(strings.TrimPrefix(key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, "", fmt.Errorf("Invalid Kite: %s", key)
	}

	kite := new(protocol.Kite)
	kite.Username = fields[1]
	kite.Environment = fields[2]
	kite.Name = fields[3]
	kite.Version = fields[4]
	kite.Region = fields[5]
	kite.Hostname = fields[6]
	kite.ID = fields[7]

	rv := new(registerValue)
	json.Unmarshal([]byte(value), rv)

	kite.PublicIP = rv.PublicIP
	kite.Port = rv.Port

	return kite, rv.KodingKey, nil
}

// WatchEtcd watches all Kite changes on etcd cluster
// and notifies registered watchers on this Kontrol instance.
func (k *Kontrol) WatchEtcd() {
getIndex:
	resp, err := k.etcd.Set("/_kontrol_get_index", "OK", 1)
	if err != nil {
		log.Critical("etcd error 1: %s", err.Error())
		time.Sleep(time.Second)
		goto getIndex
	}

	index := resp.ModifiedIndex
	log.Info("etcd: index = %d", index)

	receiver := make(chan *etcd.Response)

	go func() {
	watch:
		resp, err = k.etcd.WatchAll("/kites", index+1, receiver, nil)
		if err != nil {
			log.Critical("etcd error 2: %s", err)
			time.Sleep(time.Second)
			goto watch
		}
	}()

	// Channel is never closed.
	for resp := range receiver {
		// log.Debug("etcd: change received: %#v", resp)
		index = resp.ModifiedIndex

		// Notify deregistration events.
		if strings.HasPrefix(resp.Key, "/kites") && (resp.Action == "delete" || resp.Action == "expire") {
			kite, _, err := kiteFromEtcdKV(resp.Key, resp.Value)
			if err == nil {
				k.watcherHub.Notify(kite, protocol.Deregister)
			}
		}
	}
}

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
