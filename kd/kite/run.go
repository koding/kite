package kite

import (
	"errors"
	"koding/newKite/kd/util"
	"path/filepath"
	"syscall"
)

type Run struct{}

func NewRun() *Run {
	return &Run{}
}

func (*Run) Definition() string {
	return "Run a kite"
}

func (*Run) Exec(args []string) error {
	// Parse kite name
	if len(args) != 1 {
		return errors.New("You should give a kite name")
	}

	// Guess full kite name if short name is given
	var kiteFullName string
	suppliedName := args[0]
	_, _, err := splitVersion(kiteFullName, true)
	if err != nil {
		allKites, err := getInstalledKites(suppliedName)
		if err != nil {
			return err
		}

		if len(allKites) == 1 {
			kiteFullName = allKites[0]
		} else {
			return errors.New("More than one version is installed. Please give a full kite name.")
		}
	} else {
		kiteFullName = suppliedName
	}

	binPath, err := getBinPath(kiteFullName + ".kite")
	if err != nil {
		return err
	}

	binPath = filepath.Join(util.GetKdPath(), "kites", binPath)
	return syscall.Exec(binPath, []string{"hello", "world"}, []string{})
}
