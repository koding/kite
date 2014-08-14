package kontrol

import (
	"errors"
	"strings"
	"time"

	"github.com/coreos/go-etcd/etcd"
	"github.com/koding/kite"
	"github.com/koding/kite/kontrol/node"
)

// Storage is an interface to a kite storage.
type Storage interface {
	Get(key string) (*node.Node, error)
	Set(key, value string) error
	Update(key, value string) error
	Delete(key string) error
	Watch(key string, index uint64) (*Watcher, error)
}

type Etcd struct {
	client *etcd.Client
	log    kite.Logger
}

type Watcher struct {
	recv chan *etcd.Response
	stop chan bool
}

func (w *Watcher) Stop() {
	w.stop <- true
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

func (e *Etcd) Watch(key string, index uint64) (*Watcher, error) {
	// TODO: make the buffer configurable
	responses := make(chan *etcd.Response, 1000)
	stopChan := make(chan bool, 1)

	// Watch is blocking
	go func() {
		_, err := e.client.Watch(key, index, true, responses, stopChan)
		if err != nil {
			e.log.Warning("Remote client closed the watcher explicitly. Etcd client error: %s", err)
		}
	}()

	return &Watcher{
		recv: responses,
		stop: stopChan,
	}, nil
}

func (e *Etcd) Get(key string) (*node.Node, error) {
	resp, err := e.client.Get(key, false, true)
	if err != nil {
		return nil, err
	}

	return node.New(resp.Node), nil
}
