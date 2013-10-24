package kite

import (
	"errors"
	"flag"
	"fmt"
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
		return errors.New("You should give a full kite name. Exampe: fs-1.0.0")
	}
	fullName := flag.Arg(0)

	if _, _, err := splitVersion(fullName, false); err != nil {
		return err
	}

	installed, err := isInstalled(fullName)
	if err != nil {
		return err
	}
	if !installed {
		return fmt.Errorf("%s is not installed", fullName)
	}

	return os.RemoveAll(getBundlePath(fullName))
}

// getBundlePath returns the bundle path of a given kite.
// Example: "adsf-1.2.3" -> "~/.kd/kites/asdf-1.2.3.kite"
func getBundlePath(fullKiteName string) string {
	return filepath.Join(util.GetKdPath(), "kites", fullKiteName+".kite")
}

// isInstalled returns true if the kite is installed.
func isInstalled(fullKiteName string) (bool, error) {
	return exists(getBundlePath(fullKiteName))
}

// exists returns whether the given file or directory exists or not.
func exists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}
