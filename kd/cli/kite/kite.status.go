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

type Status struct{}

func NewStatus() *Status {
	return &Status{}
}

func (*Status) Definition() string {
	return "Displays status of a kite"
}

func getKiteNames() (error, []string) {
	kiteNames := make([]string, 0)

	folders, err := ioutil.ReadDir(".")
	if err != nil {
		return err, nil
	}

	for _, folder := range folders {
		if !folder.IsDir() {
			continue
		}
		name := folder.Name()
		ok := strings.HasSuffix(name, ".kite")
		if !ok {
			continue
		}
		kiteNames = append(kiteNames, name[:len(name)-5])
	}
	return nil, kiteNames
}

func (*Status) Exec() error {
	flag.Parse()
	kiteNames := make([]string, 0)
	var err error = nil

	if flag.NArg() > 0 {
		kiteNames = flag.Args()
	} else {
		err, kiteNames = getKiteNames()
		if err != nil {
			return err
		}
	}

	for _, kiteName := range kiteNames {
		kite := NewKite(kiteName)
		if err := kite.ShowStatus(); err != nil {
			continue
		}
	}
	return nil
}
