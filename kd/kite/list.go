package kite

import (
	"fmt"
	"io/ioutil"
	"koding/newKite/kd/util"
	"os"
	"path/filepath"
	"strings"
)

type List struct{}

func NewList() *List {
	return &List{}
}

func (*List) Definition() string {
	return "List installed kites"
}

func (*List) Exec(args []string) error {
	kites, err := getInstalledKites("")
	if err != nil {
		return err
	}

	for _, k := range kites {
		fmt.Println(k)
	}

	return nil
}

// getIntalledKites returns installed kites in .kd/kites folder.
// an empty argument returns all kites.
func getInstalledKites(kiteName string) ([]string, error) {
	kitesPath := filepath.Join(util.GetKdPath(), "kites")

	kites, err := ioutil.ReadDir(kitesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}

		return nil, err
	}

	installedKites := []string{} // to be returned
	for _, fi := range kites {
		name := fi.Name()
		if !strings.HasSuffix(name, ".kite") {
			continue
		}

		fullName := strings.TrimSuffix(name, ".kite")
		name, _, err := splitVersion(fullName, false)
		if err != nil {
			continue
		}

		if kiteName == "" || kiteName == name {
			installedKites = append(installedKites, fullName)
		}
	}

	return installedKites, nil
}
