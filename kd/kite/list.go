package kite

import (
	"fmt"
	"io/ioutil"
	"koding/newKite/kd/util"
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

func (*List) Exec() error {
	kitesPath := filepath.Join(util.GetKdPath(), "kites")
	kites, err := ioutil.ReadDir(kitesPath)
	if err != nil {
		if strings.Contains(err.Error(), "no such file or directory") {
			return nil
		}
		return err
	}
	for _, fi := range kites {
		name := fi.Name()
		if !strings.HasSuffix(name, ".kite") {
			continue
		}
		name = strings.TrimSuffix(name, ".kite")
		name, version, err := splitVersion(name, false)
		if err == nil {
			fmt.Println(name + "-" + version)
		}
	}
	return nil
}
