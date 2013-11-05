package kite

import (
	"io/ioutil"
	"koding/newkite/utils"
	"os"
	"strings"
)

func getDeps(root, startPoint string) {
	n := NewNode(startPoint, root)
	graph[startPoint] = n

	searchDir(root, startPoint)

	resolved := make(map[string]*Node, 0)
	unresolved := make(map[string]*Node, 0)
	resolve(n, resolved, unresolved)

	for _, node := range resolved {
		log.Info("%s -> ", node.Name)
		log.Info(node.Path)
	}
}

var graph = make(map[string]*Node)

func searchDir(root, sourceNode string) {
	files, _ := ioutil.ReadDir(root)
	for _, f := range files {
		if !f.IsDir() && !strings.HasSuffix(f.Name(), ".kite") {
			continue
		}

		kitepath := root + "/" + f.Name()
		manifest := kitepath + "/manifest.json"
		if _, err := os.Stat(manifest); err != nil {
			if os.IsNotExist(err) {
				log.Info("no manifest.json for %s\n", f.Name())
				return
			}
		}

		m, err := utils.ReadKiteOptions(kitepath + "/manifest.json")
		if err != nil {
			log.Info("could not read manifest", err)
			return
		}

		n := NewNode(m.Kitename, kitepath)
		graph[m.Kitename] = n
		if sourceNode != "" {
			graph[sourceNode].addEdge(n)
		}

		searchDir(kitepath, m.Kitename)
	}
}

type Node struct {
	Name  string
	Path  string
	Edges []*Node
}

func NewNode(name, kitepath string) *Node {
	return &Node{
		Name:  name,
		Path:  kitepath,
		Edges: make([]*Node, 0),
	}
}

func (n *Node) addEdge(nd *Node) {
	n.Edges = append(n.Edges, nd)
}

func resolve(n *Node, resolved, unresolved map[string]*Node) {
	unresolved[n.Name] = n

	for _, edge := range n.Edges {
		if _, ok := unresolved[edge.Name]; ok {
			log.Fatalf("cycle %s <-> %s\n", n.Name, edge.Name)
		}

		if _, ok := resolved[edge.Name]; !ok {
			resolve(edge, resolved, unresolved)
		}
	}

	delete(unresolved, n.Name)
	resolved[n.Name] = n
}
