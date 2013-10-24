// Package kite implements the "kd kite" sub-commands.
package kite

import (
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

type Kite struct {
	KiteName string
	Folder   string
	// gives path for kite executable
	KiteExecutable string
}

func NewKite(kiteName string) *Kite {
	folder := kiteName + ".kite"
	kiteExecutable := "./" + filepath.Join(filepath.Join(folder, kiteName+"-kite"))
	return &Kite{
		KiteName:       kiteName,
		Folder:         folder,
		KiteExecutable: kiteExecutable,
	}
}
