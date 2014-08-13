package command

import (
	"os"

	"github.com/koding/kite"
	"github.com/mitchellh/cli"
)

const (
	AppName    = "kitectl"
	AppVersion = "0.0.8"
)

var (
	DefaultKiteClient = defaultKiteClient()
	DefaultUi         = defaultUi()
)

func defaultKiteClient() *kite.Kite {
	return kite.New(AppName, AppVersion)
}

func defaultUi() cli.Ui {
	return &cli.ColoredUi{
		InfoColor:  cli.UiColorYellow,
		ErrorColor: cli.UiColorRed,
		Ui: &cli.BasicUi{
			Reader:      os.Stdin,
			Writer:      os.Stdout,
			ErrorWriter: os.Stdout,
		},
	}
}
