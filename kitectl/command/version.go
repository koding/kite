package command

import (
	"strings"

	"github.com/mitchellh/cli"
)

type VersionCommand struct {
	Version string
	Ui      cli.Ui
}

func (c *VersionCommand) Synopsis() string {
	return "Shows kitectl version"
}

func (c *VersionCommand) Help() string {
	helpText := `
Usage: kitectl version

  Shows kitectl version.
`
	return strings.TrimSpace(helpText)
}

func (c *VersionCommand) Run(_ []string) int {
	c.Ui.Output(c.Version)

	return 0
}
