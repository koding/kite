package cmd

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
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

	// User is allowed to enter kite name in these forms: "fs" or "github.com/koding/fs.kite/1.0.0"
	suppliedName := args[0]

	installedKites, err := getInstalledKites(suppliedName)
	if err != nil {
		return err
	}

	var matched []*InstalledKite

	for _, ik := range installedKites {
		if strings.TrimSuffix(ik.Repo, ".kite") == strings.TrimSuffix(suppliedName, ".kite") {
			matched = append(matched, ik)
		}
	}

	if len(matched) == 0 {
		for _, ik := range installedKites {
			if ik.String() == suppliedName {
				matched = append(matched, ik)
			}
		}
	}

	if len(matched) == 0 {
		return errors.New("Kite not found")
	}

	if len(matched) > 1 {
		return errors.New("More than one version is installed. Please give a full kite name as: domain/user/repo/version")
	}

	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return err
	}

	binPath := filepath.Join(kiteHome, "kites", matched[0].BinPath())
	return syscall.Exec(binPath, args, os.Environ())
}
