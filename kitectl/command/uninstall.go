package command

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type Uninstall struct {
	Ui cli.Ui
}

func NewUninstall() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Uninstall{
			Ui: DefaultUi,
		}, nil
	}
}

func (c *Uninstall) Synopsis() string {
	return "Uninstalls a kite"
}

func (c *Uninstall) Help() string {
	helpText := `
Usage: kitectl uninstall kitename

  Uninstall the given kite. Example kitename: github.com/koding/fs.kite/1.0.0
`
	return strings.TrimSpace(helpText)
}

func (c *Uninstall) Run(args []string) int {

	if len(args) != 1 {
		c.Ui.Output(c.Help())
		return 1
	}
	fullName := args[0]

	installed, err := isInstalled(fullName)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	if !installed {
		c.Ui.Error(fmt.Sprintf("%s is not installed", fullName))
		return 1
	}

	bundlePath, err := getBundlePath(fullName)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	err = os.RemoveAll(bundlePath)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	return 0
}

// getBundlePath returns the bundle path of a given kite.
// Example: "adsf-1.2.3" -> "~/.kd/kites/asdf-1.2.3.kite"
func getBundlePath(fullKiteName string) (string, error) {
	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(kiteHome, "kites", fullKiteName), nil
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
