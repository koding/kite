package cli

import (
	"archive/tar"
	"bufio"
	"code.google.com/p/go.crypto/ssh/terminal"
	"compress/gzip"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"koding/newkite/protocol"
	"koding/tools/process"
	"log"
	"net/http"
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

/****************************************

kd kite install

****************************************/

type Install struct{}

func NewInstall() *Install {
	return &Install{}
}

func (*Install) Definition() string {
	return "Install kite from Koding repository"
}

const s3URL = "http://koding-kites.s3.amazonaws.com/"

func (*Install) Exec() error {
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("You should give a kite name")
	}

	// Generate download URL
	kiteName := flag.Arg(0)
	kiteURL := s3URL + kiteName + ".kite.tar.gz"
	log.Println(kiteURL)

	// Make download request
	fmt.Println("Downloading...")
	res, err := http.Get(kiteURL)
	if err != nil {
		return err
	}
	defer res.Body.Close()

	// Extract gzip
	gz, err := gzip.NewReader(res.Body)
	if err != nil {
		return err
	}
	defer gz.Close()

	// Extract tar
	tempKitePath, err := ioutil.TempDir("", "koding-kite-")
	log.Println("Created temp dir:", tempKitePath)
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempKitePath)
	err = extractTar(gz, tempKitePath)
	if err != nil {
		return err
	}

	// Move kite from tmp to kites folder (~/.kd/kites)
	tempKitePath = filepath.Join(tempKitePath, kiteName+".kite")
	kitesPath := filepath.Join(getKdPath(), "kites")
	os.MkdirAll(kitesPath, 0700)
	kitePath := filepath.Join(kitesPath, kiteName+".kite")
	log.Println("Moving from:", tempKitePath, "to:", kitePath)
	err = os.Rename(tempKitePath, kitePath)
	if err != nil {
		return err
	}

	fmt.Println("Done.")
	return nil
}

// extractTar reads from the io.Reader and writes the files into the directory.
func extractTar(r io.Reader, dir string) error {
	tr := tar.NewReader(r)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			// end of tar archive
			break
		}
		if err != nil {
			return err
		}

		fi := hdr.FileInfo()
		name := fi.Name()
		path := filepath.Join(dir, name)

		// TODO make the binary under /bin executable
		// TODO assert contents of the tar file, it must contain online one directory named kitename-0.0.1.kite

		if fi.IsDir() {
			os.MkdirAll(path, 0700)
		} else {
			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
			if err != nil {
				return err
			}

			if _, err := io.Copy(f, tr); err != nil {
				return err
			}
		}
	}
	return nil
}

/****************************************

kd run

****************************************/

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

/****************************************

kd stop

****************************************/

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

/****************************************

kd kite status

****************************************/

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

/****************************************

kd create

****************************************/

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

/****************************************

kd pack pkg

****************************************/

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

/****************************************

kd deploy remotessh

****************************************/

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
