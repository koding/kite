package kontrol

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coreos/go-etcd/etcd"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
)

// Node is a wrapper around an etcd node to provide additional
// functionality around kites.
type Node struct {
	Node *etcd.Node
}

// New returns a new initialized node with the given etcd node.
func NewNode(node *etcd.Node) *Node {
	return &Node{
		Node: node,
	}
}

// HasValue returns true if the give node has a non-empty value
func (n *Node) HasValue() bool {
	return n.Node.Value != ""
}

// Flatten converts the recursive etcd directory structure to a flat one that
// contains all kontrolNodes
func (n *Node) Flatten() []*Node {
	nodes := make([]*Node, 0)
	for _, node := range n.Node.Nodes {
		if node.Dir {
			nodes = append(nodes, NewNode(node).Flatten()...)
			continue
		}

		nodes = append(nodes, NewNode(node))
	}

	return nodes
}

// Kite returns a single kite gathered from the key and the value for the
// current node.
func (n *Node) Kite() (*protocol.KiteWithToken, error) {
	kite, err := n.KiteFromKey()
	if err != nil {
		return nil, err
	}

	val, err := n.Value()
	if err != nil {
		return nil, err
	}

	return &protocol.KiteWithToken{
		Kite:  *kite,
		URL:   val.URL,
		KeyID: val.KeyID,
	}, nil
}

// KiteFromKey returns a *protocol.Kite from an etcd key. etcd key is like:
// "/kites/devrim/env/mathworker/1/localhost/tardis.local/id"
func (n *Node) KiteFromKey() (*protocol.Kite, error) {
	// TODO replace "kites" with KitesPrefix constant
	fields := strings.Split(strings.TrimPrefix(n.Node.Key, "/"), "/")
	if len(fields) != 8 || (len(fields) > 0 && fields[0] != "kites") {
		return nil, fmt.Errorf("kontrol: invalid kite %s", n.Node.Key)
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

// Value returns the value associated with the current node.
func (n *Node) Value() (kontrolprotocol.RegisterValue, error) {
	var rv kontrolprotocol.RegisterValue
	err := json.Unmarshal([]byte(n.Node.Value), &rv)
	return rv, err
}

// Kites returns a list of kites that are gathered by collecting recursively
// all nodes under the current node.
func (n *Node) Kites() (Kites, error) {
	// Get all nodes recursively.
	nodes := n.Flatten()

	// Convert etcd nodes to kites.
	var err error
	kites := make(Kites, len(nodes))
	for i, n := range nodes {
		kites[i], err = n.Kite()
		if err != nil {
			return nil, err
		}
	}

	return kites, nil
}
