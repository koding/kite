package create

import (
	"core"
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
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
	var port = flag.Int("port", 4010, "port number")
	if len(os.Args) == 0 {
		return errors.New("You should give a directory name")
	}
	flag.Parse()
	fmt.Printf("port:%d\n", *port)
	fmt.Println("Creating kite\n")
	folder := os.Args[0]
	err := os.Mkdir(folder, 0755)
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
	err := ioutil.WriteFile(path, []byte("buff"), 0755)
	if err != nil {
		return err
	}

	return nil
}
