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

type RemoteSSH struct{}

func NewRemoteSSH() *RemoteSSH {
	return &RemoteSSH{}
}

func (*RemoteSSH) Definition() string {
	return "Deploys kite to a remote location with ssh"
}

// this function is a scaffold, will be expanded
func (*RemoteSSH) Exec() error {
	flag.Parse()
	if flag.NArg() < 2 {
		return errors.New("You should give a kite name and a host name")
	}
	kiteName := flag.Arg(0)
	fmt.Println(kiteName)
	hostName := flag.Arg(1)
	fmt.Println("Enter username")
	r := bufio.NewReader(os.Stdin)
	username, _, _ := r.ReadLine()
	fmt.Println("Enter password")
	password, _ := terminal.ReadPassword(int(os.Stdin.Fd()))
	client := NewSSHClient(hostName)
	client.SetCredentialAuth(string(username), string(password))
	err, session := client.newSession()
	if err != nil {
		return err
	}
	defer session.Close()
	out, err := session.Execute("/usr/bin/whoami")
	fmt.Println(out)
	if err != nil {
		return err
	}
	return nil
}
