package create

import (
	"errors"
	"flag"
	"io/ioutil"
	"koding/newkite/cli/core"
	"os"
	"path/filepath"
)

func New() *core.Command {
	return &core.Command{Help: Help, Exec: Exec}
}

func Help() string {
	return "Creates a kite from kite skeleton"
}

func Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a directory name")
	}
	folder := flag.Arg(0)
	err := os.Mkdir(folder, 0755)
	if os.IsExist(err) {
		return errors.New("Kite already exists")
	}
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(folder, "bin"), 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(folder, "resources"), 0755)
	if err != nil {
		return err
	}
	createManifest(folder)
	return nil
}

func createManifest(folder string) error {
	path := filepath.Join(folder, "manifest.json")
	err := ioutil.WriteFile(path, []byte("buff\n"), 0755)
	if err != nil {
		return err
	}

	return nil
}
