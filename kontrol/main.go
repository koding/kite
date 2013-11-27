package main

import (
	"errors"
	"fmt"
	logging "github.com/op/go-logging"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/dnode"
	"koding/newkite/kite"
	"koding/newkite/peers"
	"koding/newkite/protocol"
	"koding/tools/config"
	stdlog "log"
	"net"
	"os"
	"strconv"
	"time"
)

const (
	HEARTBEAT_INTERVAL = time.Millisecond * 1000
	HEARTBEAT_DELAY    = time.Millisecond * 2000
)

var log = logging.MustGetLogger("Kontrol")

// Storage is an interface that encapsulates basic operations on the kite
// struct. ID is an unique string that belongs to the kite.
type Storage interface {
	// Add inserts the kite into the storage with the kite.ID key. If there
	// is already a kite available with this id, it should update/replace it.
	Add(kite *models.Kite)

	// Get returns the specified kite struct with the given id
	Get(id string) *models.Kite

	// Remove deletes the kite with the given id
	Remove(id string)

	// Has checks whether the kite with the given id exist
	Has(id string) bool

	// Size returns the total number of kites in the storage
	Size() int

	// List returns a slice of all kites in the storage
	List() []*models.Kite
}

type Kontrol struct {
	storage  Storage
	watchers *watchers
}

func NewKontrol() *Kontrol {
	return &Kontrol{
		storage:  peers.New(),
		watchers: newWatchers(),
	}
}

func main() {
	kontrol := NewKontrol()
	kontrol.setupLogging()

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
	k.HandleFunc("watchKites", kontrol.handleWatchKites)

	go kontrol.heartBeatChecker()

	k.Run()
}

func (k *Kontrol) setupLogging() {
	log.Module = "Kontrol"
	logging.SetFormatter(logging.MustStringFormatter("%{level:-8s} ▶ %{message}"))
	stderrBackend := logging.NewLogBackend(os.Stderr, "", stdlog.LstdFlags|stdlog.Lshortfile)
	stderrBackend.Color = true
	syslogBackend, _ := logging.NewSyslogBackend(log.Module)
	logging.SetBackend(stderrBackend, syslogBackend)

	log.Info("started")
}

// heartBeatChecker removes dead kites from the DB.
func (k *Kontrol) heartBeatChecker() {
	for {
		k.removeDeadKites()
		time.Sleep(HEARTBEAT_INTERVAL)
	}
}

func (k *Kontrol) removeDeadKites() {
	for _, kite := range k.storage.List() {
		// Delay is needed to fix network delays, otherwise kites are
		// marked as death even if they are sending pings to us.
		if time.Now().UTC().Before(kite.UpdatedAt.Add(HEARTBEAT_DELAY)) {
			continue // still alive, pick up the next one
		}

		log.Info("Removing dead Kite: %#v", kite)

		k.storage.Remove(kite.ID)

		// only delete from jVMs when all kites to that user is died.
		if !k.kitesExistsForUser(kite.Username) {
			deleteFromVM(kite.Username)
		}
	}
}

func (k *Kontrol) handleRegister(r *kite.Request) (interface{}, error) {
	log.Info("Register request from: %#v", r.RemoteKite.Kite)

	// Only accept requests with kodingKey because we need this info
	// for generating tokens for this kite.
	if r.Authentication.Type != "kodingKey" {
		return nil, fmt.Errorf("Unexpected authentication type: %s", r.Authentication.Type)
	}

	if r.Authentication.Key == "" {
		return nil, errors.New("Invalid Koding Key")
	}

	if r.RemoteKite.ID == "" {
		return nil, errors.New("Invalid Kite ID")
	}

	// Prevent registration with same ID.
	kite := k.storage.Get(r.RemoteKite.ID)
	if kite == nil {
		kite = k.addKite(r)
	}

	log.Info("Kite registered: %#v", kite.Kite)

	// Request heartbeat from the Kite.
	err := k.requestHeartbeat(&kite.Kite, r.RemoteKite)
	if err != nil {
		return nil, err
	}

	// send response back to the kite, also identify him with the new name
	response := protocol.RegisterResult{
		Result:   protocol.AllowKite,
		Username: kite.Username,
		PublicIP: kite.PublicIP,
	}

	go k.watchers.Notify(&kite.Kite)

	return response, nil
}

func (k *Kontrol) addKite(r *kite.Request) *models.Kite {
	kite := modelhelper.NewKite()
	kite.Kite = r.RemoteKite.Kite
	kite.Username = r.Username
	kite.KodingKey = r.Authentication.Key

	if r.RemoteKite.PublicIP == "" {
		kite.PublicIP, _, _ = net.SplitHostPort(r.RemoteAddr)
	}

	// Deregister the Kite on disconnect.
	r.RemoteKite.OnDisconnect(func() {
		log.Info("Deregistering Kite: %s", r.RemoteKite.ID)
		k.storage.Remove(r.RemoteKite.ID)

		// Delete from jVMs when all kites to that user is died.
		if !k.kitesExistsForUser(r.RemoteKite.Username) {
			deleteFromVM(r.RemoteKite.Username)
		}
	})

	k.storage.Add(kite)
	return kite
}

func (k *Kontrol) requestHeartbeat(kite *protocol.Kite, remote *kite.RemoteKite) error {
	updateKite := func(p *dnode.Partial) {
		err := k.updateKite(kite.ID)
		if err != nil {
			log.Warning("Came heartbeat but the Kite is not registered. Dropping it's connection to prevent bad things happening. It may be an indication that the heartbeat delay is too short.")
			remote.Close()
		}
	}
	heartbeatArgs := []interface{}{
		HEARTBEAT_INTERVAL / time.Second,
		dnode.Callback(updateKite),
	}
	_, err := remote.Call("heartbeat", heartbeatArgs)
	return err
}

func (k *Kontrol) updateKite(id string) error {
	kite := k.storage.Get(id)
	if kite == nil {
		return errors.New("Kite not registered")
	}

	kite.UpdatedAt = time.Now().UTC().Add(HEARTBEAT_INTERVAL)
	k.storage.Add(kite)
	return nil
}

func (k *Kontrol) handleGetKites(r *kite.Request) (interface{}, error) {
	var query KontrolQuery
	err := r.Args.Unmarshal(&query)
	if err != nil {
		return nil, err
	}

	// We do not allow access to other's kites for now.
	if r.Username != query.Username {
		return nil, errors.New("Not your Kite")
	}

	return query.Run()
}

func (k *Kontrol) handleWatchKites(r *kite.Request) (interface{}, error) {
	var args []*dnode.Partial
	err := r.Args.Unmarshal(&args)
	if err != nil {
		return nil, err
	}

	if len(args) != 2 {
		return nil, errors.New("Invalid number of arguments")
	}

	var query KontrolQuery
	err = args[0].Unmarshal(&query)
	if err != nil {
		return nil, err
	}

	var callback dnode.Function
	err = args[1].Unmarshal(&callback)
	if err != nil {
		return nil, err
	}

	// We do not allow access to other's kites for now.
	if r.Username != query.Username {
		return nil, errors.New("Not your Kite")
	}

	k.watchers.RegisterWatcher(r.RemoteKite, &query, callback)
	return nil, nil
}

func deleteFromVM(username string) error {
	if username == "" {
		return errors.New("deleting local vm err: empty username is passed")
	}

	hostnameAlias := "local-" + username
	err := modelhelper.DeleteVM(hostnameAlias)
	if err != nil {
		return fmt.Errorf("deleting local vm err:", err)
	}
	return nil
}

func (k *Kontrol) kitesExistsForUser(username string) bool {
	found := false

	for _, kite := range k.storage.List() {
		if kite.Username == username {
			found = true
		}
	}

	return found
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
