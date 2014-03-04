package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"syscall"

	"github.com/koding/kite/kitekey"
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
	if len(args) == 0 {
		return errors.New("You should give a kite name")
	}

	// Guess full kite name if short name is given
	var kiteFullName string

	// User is allowed to enter kite name in these forms: "fs", "fs-0.0.1"
	suppliedName := args[0]

	_, _, err := splitVersion(suppliedName, false)
	if err != nil {
		allKites, err := getInstalledKites(suppliedName)
		if err != nil {
			return err
		}

		if len(allKites) == 0 {
			return errors.New("Kite not found")
		}

		if len(allKites) > 1 {
			return errors.New("More than one version is installed. Please give a full kite name.")
		}

		kiteFullName = allKites[0]
	} else {
		kiteFullName = suppliedName
	}

	binPath, err := getBinPath(kiteFullName + ".kite")
	if err != nil {
		return err
	}

	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return err
	}

	binPath = filepath.Join(kiteHome, "kites", binPath)
	return syscall.Exec(binPath, args, os.Environ())
}
