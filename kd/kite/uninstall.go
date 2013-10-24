package kite

import (
	"errors"
	"flag"
	"koding/newKite/kd/util"
	"os"
	"path/filepath"
)

type Uninstall struct{}

func NewUninstall() *Uninstall {
	return &Uninstall{}
}

func (*Uninstall) Definition() string {
	return "Uninstall a kite"
}

func (*Uninstall) Exec() error {
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("You should give a kite name")
	}
	name := flag.Arg(0)

	path := filepath.Join(util.GetKdPath(), "kites", name+".kite")
	return os.RemoveAll(path)
}
