package kontrol

import (
	"errors"
	"strings"

	"github.com/coreos/etcd/store"
	"github.com/coreos/go-etcd/etcd"
)

type Storage interface {
	Get(key string) (*Node, error)
}

type Etcd struct {
	client *etcd.Client
}

func NewEtcd() (*Etcd, error) {
	machines := []string{"http://127.0.0.1:4001"}
	client := etcd.NewClient(machines)

	ok := client.SetCluster(machines)
	if !ok {
		return nil, errors.New("cannot connect to etcd cluster: " + strings.Join(machines, ","))
	}

	return &Etcd{
		client: client,
	}, nil
}

func (e *Etcd) Get(key string) (*Node, error) {
	resp, err := e.client.Get(key, false, true)
	if err != nil {
		return nil, err
	}

	nodeExtern := convertNodeToNodeExtern(resp.Node)

	return NewNode(nodeExtern), nil
}

func convertNodeToNodeExtern(node *etcd.Node) *store.NodeExtern {
	s := &store.NodeExtern{
		Key:           node.Key,
		Value:         &node.Value,
		Dir:           node.Dir,
		Expiration:    node.Expiration,
		TTL:           node.TTL,
		ModifiedIndex: node.ModifiedIndex,
		CreatedIndex:  node.CreatedIndex,
		Nodes:         make(store.NodeExterns, len(node.Nodes)),
	}

	for i, n := range node.Nodes {
		s.Nodes[i] = convertNodeToNodeExtern(n)
	}

	return s
}
