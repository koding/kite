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

func (k *Kite) GetPid() (int, error) {
	pid, err := ioutil.ReadFile(filepath.Join(k.Folder, ".pid"))
	// This is not an error, pid is not written yet
	if err != nil {
		return 0, nil
	}
	p, err := strconv.Atoi(string(pid))
	if err != nil {
		return 0, err
	}
	return p, nil
}

func (k *Kite) SetPid(pid int) error {
	p := strconv.Itoa(pid)
	pidfile := filepath.Join(k.Folder, ".pid")
	if err := ioutil.WriteFile(pidfile, []byte(p), 0755); err != nil {
		return err
	}
	return nil
}

func (k *Kite) Build() error {
	gofile := filepath.Join(k.Folder, k.KiteName+".go")
	sout, err := exec.Command("go", "build", gofile).CombinedOutput()
	fmt.Printf(string(sout))
	if err != nil {
		return err
	}
	return exec.Command("mv", k.KiteName, k.KiteExecutable).Run()
}

func (k *Kite) Start() error {
	if err := k.Build(); err != nil {
		return err
	}

	cmd := exec.Command(k.KiteExecutable)
	if err := cmd.Start(); err != nil {
		return err
	}
	return k.SetPid(cmd.Process.Pid)
}

func (k *Kite) Exists() bool {
	inf, err := os.Stat(k.Folder)
	if err != nil {
		return false
	}
	return inf.IsDir()
}

func (k *Kite) Running() (error, bool) {
	pid, err := k.GetPid()
	if err != nil {
		return err, false
	}
	if pid != 0 {
		succ := process.CheckPid(pid)
		if succ == nil {
			return nil, true
		}
	}
	return nil, false
}

func (k *Kite) Kill() error {
	pid, err := k.GetPid()
	if err != nil {
		return err
	}
	if pid != 0 {
		return process.KillPid(pid)
	}
	return nil
}

func (k *Kite) ShowStatus() error {
	if !k.Exists() {
		return fmt.Errorf("There is no kite folder named %s.kite", k.KiteName)
	}
	err, ok := k.Running()
	if err != nil {
		return err
	}
	fmt.Printf("  %s:\n", k.KiteName)
	if ok {
		fmt.Printf("    state: running\n")
	} else {
		fmt.Printf("    state: not running\n")
	}
	pid, err := k.GetPid()
	if err != nil {
		return err
	}
	if pid != 0 {
		fmt.Printf("    pid: %d\n", pid)
	}
	return nil
}

func (k *Kite) createManifest() error {
	path := filepath.Join(k.Folder, "manifest.json")
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	options := &protocol.Options{
		Username:     currUser.Name,
		Kitename:     k.KiteName,
		LocalIP:      "",
		PublicIP:     "",
		Port:         "",
		Version:      "0.1",
		Dependencies: "",
	}
	body, err := json.MarshalIndent(options, "", "    ")
	if err != nil {
		return err
	}
	return ioutil.WriteFile(path, []byte(body), 0755)
}

func (k *Kite) Create() error {
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	skelPath := filepath.Join(currUser.HomeDir, "/.kd/skel.go")
	kitePath := filepath.Join(k.Folder, k.KiteName+".go")
	if err = cp(skelPath, kitePath); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(k.Folder, "bin"), 0755); err != nil {
		return err
	}
	if err = os.MkdirAll(filepath.Join(k.Folder, "resources"), 0755); err != nil {
		return err
	}
	return k.createManifest()
}

func (k *Kite) createPkg() error {
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	// User will create root:staff files, so we need to check if
	// the user is root
	if currUser.Username != "root" {
		return errors.New("You should be root to pack pkg files")
	}
	tmppath, err := ioutil.TempDir("", "")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmppath)
	execpath := filepath.Join(tmppath, "/usr/local/bin")
	if err = os.MkdirAll(execpath, 0755); err != nil {
		return nil
	}
	// copying the executable into the package
	execName := filepath.Join(execpath, k.KiteName+"-kite")
	if err = cp(k.KiteExecutable, execName); err != nil {
		return err
	}
	// changing the permissions and owner in order to create the package
	if err = exec.Command("chown", "-R", "root:staff", tmppath).Run(); err != nil {
		return err
	}
	if err = exec.Command("chmod", "-R", "755", tmppath).Run(); err != nil {
		return err
	}
	fmt.Println("packaging kite")
	return exec.Command("pkgbuild", "--identifier", "com."+k.KiteName+"-kite", "--root", tmppath, k.KiteName+".pkg").Run()
}

func cp(src, dst string) error {
	s, err := os.Open(src)
	if err != nil {
		return err
	}
	defer s.Close()
	d, err := os.Create(dst)
	if err != nil {
		return err
	}
	if _, err := io.Copy(d, s); err != nil {
		d.Close()
		return err
	}
	return d.Close()
}
