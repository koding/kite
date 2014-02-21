package cmd

import (
	"errors"
	"fmt"
	"github.com/koding/kite/kitekey"
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

func (*Uninstall) Exec(args []string) error {
	if len(args) != 1 {
		return errors.New("You should give a full kite name. Exampe: fs-1.0.0")
	}
	fullName := args[0]

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

	bundlePath, err := getBundlePath(fullName)
	if err != nil {
		return err
	}

	return os.RemoveAll(bundlePath)
}

// getBundlePath returns the bundle path of a given kite.
// Example: "adsf-1.2.3" -> "~/.kd/kites/asdf-1.2.3.kite"
func getBundlePath(fullKiteName string) (string, error) {
	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(kiteHome, "kites", fullKiteName+".kite"), nil
}

// isInstalled returns true if the kite is installed.
func isInstalled(fullKiteName string) (bool, error) {
	bundlePath, err := getBundlePath(fullKiteName)
	if err != nil {
		return false, err
	}
	return exists(bundlePath)
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
