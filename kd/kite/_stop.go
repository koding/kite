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

type Stop struct{}

func NewStop() *Stop {
	return &Stop{}
}

func (*Stop) Definition() string {
	return "Stops a running kite"
}

func (*Stop) Exec() error {
	flag.Parse()
	if flag.NArg() == 0 {
		return errors.New("You should give a kite name")
	}

	kiteName := flag.Arg(0)
	kite := NewKite(kiteName)
	if !kite.Exists() {
		return fmt.Errorf("There is no kite folder named %s.kite", kiteName)
	}
	err, ok := kite.Running()
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("Kite is not running")
	}
	if err = kite.Kill(); err != nil {
		return err
	}
	fmt.Println("Stopped kite")
	return kite.SetPid(0)
}
