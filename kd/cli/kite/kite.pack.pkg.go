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

type Pkg struct{}

func NewPkg() *Pkg {
	return &Pkg{}
}

func (*Pkg) Definition() string {
	return "Create Mac OSX compatible .pkg file"
}

func (*Pkg) Exec() error {
	flag.Parse()
	if flag.NArg() == 0 {
		return errors.New("You should give a kite name")
	}
	kiteName := flag.Arg(0)
	kite := NewKite(kiteName)
	if !kite.Exists() {
		return fmt.Errorf("There is no kite folder named %s.kite", kiteName)
	}
	fmt.Println("building kite")
	if err := kite.Build(); err != nil {
		return err
	}
	if err := kite.createPkg(); err != nil {
		return err
	}
	fmt.Printf("kite %s packaged as %s\n", kiteName, kiteName+".pkg")
	return nil
}
