package kite

import (
	"errors"
	"flag"
	"io"
	"io/ioutil"
	"os"
	"path/filepath"
)

type Create struct{}

func NewCreate() *Create {
	return &Create{}
}

func (c Create) Help() string {
	return "Creates a kite from kite skeleton"
}

func (c Create) Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a kite name")
	}
	kiteName := flag.Arg(0)
	folder := kiteName + ".kite"
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
	err = createManifest(folder)
	if err != nil {
		return err
	}
	err = createKite(folder, kiteName)
	if err != nil {
		return err
	}
	return nil
}

func createManifest(folder string) error {
	path := filepath.Join(folder, "manifest.json")
	return ioutil.WriteFile(path, []byte("buff\n"), 0755)
}

func createKite(folder, kiteName string) error {
	return cp(os.Getenv("HOME")+"/.kd/skel.go", filepath.Join(folder, kiteName+".go"))
}

func cp(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}
