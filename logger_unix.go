// +build !windows

package kite

import (
	"os"
	"os/signal"
	"syscall"
)

// SetupSignalHandler listens to signals and toggles the log level to DEBUG
// mode when it received a SIGUSR2 signal. Another SIGUSR2 toggles the log
// level back to the old level.
func (k *Kite) SetupSignalHandler() {
	c := make(chan os.Signal, 1)

	signal.Notify(c, syscall.SIGUSR2)
	go func() {
		for s := range c {
			k.Log.Info("Got signal: %s", s)

			if debugMode {
				// toogle back to old settings.
				k.Log.Info("Disabling debug mode")
				if k.SetLogLevel == nil {
					k.Log.Error("SetLogLevel is not defined")
					continue
				}

				k.SetLogLevel(getLogLevel())
				debugMode = false
			} else {
				k.Log.Info("Enabling debug mode")
				if k.SetLogLevel == nil {
					k.Log.Error("SetLogLevel is not defined")
					continue
				}

				k.SetLogLevel(DEBUG)
				debugMode = true
			}
		}
	}()
}
