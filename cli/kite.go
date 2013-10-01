package cli

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"koding/newkite/protocol"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
)

type Run struct{}

func NewRun() *Run {
	return &Run{}
}

func (r Run) Definition() string {
	return "Runs the kite"
}

func (r Run) Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a kite name")
	}
	kiteName := flag.Arg(0)
	folder := kiteName + ".kite"
	fmt.Println("go" + " run " + filepath.Join(folder, kiteName+".go"))
	cmd := exec.Command("go", "run", filepath.Join(folder, kiteName+".go"))
	err := cmd.Start()
	if err != nil {
		return err
	}
	// TODO status of kite should be checked
	fmt.Println("Started kite")
	return nil
}

type Create struct{}

func NewCreate() *Create {
	return &Create{}
}

func (c Create) Definition() string {
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
	err = createManifest(folder, kiteName)
	if err != nil {
		return err
	}
	err = createKite(folder, kiteName)
	if err != nil {
		return err
	}
	return nil
}

func createManifest(folder, kiteName string) error {
	path := filepath.Join(folder, "manifest.json")
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	options := &protocol.Options{
		Username:     currUser.Name,
		Kitename:     kiteName,
		LocalIP:      "",
		PublicIP:     "",
		Port:         "",
		Version:      "0.1",
		Dependencies: "",
	}
	body, err := json.MarshalIndent(options, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, []byte(body), 0755)
}

func createKite(folder, kiteName string) error {
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	skelPath := filepath.Join(currUser.HomeDir, "/.kd/skel.go")
	kitePath := filepath.Join(folder, kiteName+".go")
	return cp(skelPath, kitePath)
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
