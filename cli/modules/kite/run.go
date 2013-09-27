package kite

import (
	"errors"
	"flag"
	"fmt"
	"os/exec"
	"path/filepath"
)

type Run struct{}

func NewRun() *Run {
	return &Run{}
}

func (r Run) Help() string {
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
