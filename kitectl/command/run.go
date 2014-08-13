package command

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/koding/kite/kitekey"
	"github.com/mitchellh/cli"
)

type Run struct {
	Ui cli.Ui
}

func NewRun() cli.CommandFactory {
	return func() (cli.Command, error) {
		return &Run{Ui: DefaultUi}, nil
	}
}

func (c *Run) Synopsis() string {
	return "Runs a kite"
}

func (c *Run) Help() string {
	helpText := `
Usage: kitectl run kitename

  Runs the given kite.
`
	return strings.TrimSpace(helpText)
}

func (c *Run) Run(args []string) int {

	// Parse kite name
	if len(args) == 0 {
		c.Ui.Output(c.Help())
		return 1
	}

	// User is allowed to enter kite name in these forms: "fs" or "github.com/koding/fs.kite/1.0.0"
	suppliedName := args[0]

	installedKites, err := getInstalledKites(suppliedName)
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
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
		c.Ui.Error("Kite not found")
		return 1
	}

	if len(matched) > 1 {
		c.Ui.Error("More than one version is installed. Please give a full kite name as: domain/user/repo/version")
		return 1
	}

	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	binPath := filepath.Join(kiteHome, "kites", matched[0].BinPath())
	err = syscall.Exec(binPath, args, os.Environ())
	if err != nil {
		c.Ui.Error(err.Error())
		return 1
	}

	return 0
}
