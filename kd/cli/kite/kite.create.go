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

type Create struct{}

func NewCreate() *Create {
	return &Create{}
}

func (*Create) Definition() string {
	return "Creates a kite from kite skeleton"
}

func (*Create) Exec() error {
	flag.Parse()
	if flag.NArg() == 0 {
		return errors.New("You should give a kite name")
	}

	kiteName := flag.Arg(0)
	kite := NewKite(kiteName)

	err := os.Mkdir(kite.Folder, 0755)
	if os.IsExist(err) {
		return errors.New("Kite already exists")
	}
	if err != nil {
		return err
	}

	return kite.Create()
}
