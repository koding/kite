package kite

import (
	// "bufio"
	// "code.google.com/p/go.crypto/ssh/terminal"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"koding/newkite/protocol"
	"koding/tools/process"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strconv"
	"strings"
)

type Run struct{}

func NewRun() *Run {
	return &Run{}
}

func (*Run) Definition() string {
	return "Runs the kite"
}

func (*Run) Exec() error {
	flag.Parse()
	if flag.NArg() == 0 {
		return errors.New("You should give a kite name")
	}
	kiteName := flag.Arg(0)
	kite := NewKite(kiteName)
	if !kite.Exists() {
		return fmt.Errorf("There is no kite folder named %s.kite", kiteName)
	}
	err, isRunning := kite.Running()
	if err != nil {
		return err
	}
	if isRunning {
		return fmt.Errorf("The kite is already running")
	}
	if err = kite.Start(); err != nil {
		return err
	}
	fmt.Println("Started kite")
	return nil
}
