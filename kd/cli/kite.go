package cli

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
)

/****************************************

kd run

****************************************/

type Run struct{}

func NewRun() *Run {
	return &Run{}
}

func (Run) Definition() string {
	return "Runs the kite"
}

func (Run) Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a kite name")
	}
	kiteName := flag.Arg(0)
	folder := kiteName + ".kite"
	if !kiteExists(folder) {
		return fmt.Errorf("There is no kite named %s", kiteName)
	}
	err, ok := kiteRunning(folder)
	if err != nil {
		return err
	}
	if ok {
		return fmt.Errorf("The kite is already running")
	}
	err = startKite(folder, kiteName)
	if err != nil {
		return nil
	}
	fmt.Println("Started kite")
	return nil
}

func getKitePid(folder string) (int, error) {
	pid, err := ioutil.ReadFile(filepath.Join(folder, ".pid"))
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

func setKitePid(folder string, pid int) error {
	p := strconv.Itoa(pid)
	err := ioutil.WriteFile(filepath.Join(folder, ".pid"), []byte(p), 0755)
	if err != nil {
		return err
	}
	return nil
}

func startKite(folder, kiteName string) error {
	cmd := exec.Command("go", "build", filepath.Join(folder, kiteName+".go"))
	err := cmd.Run()
	if err != nil {
		return nil
	}
	cmd = exec.Command("mv", kiteName, filepath.Join(folder, kiteName))
	err = cmd.Run()
	if err != nil {
		return err
	}
	cmd = exec.Command("./" + filepath.Join(folder, kiteName))
	err = cmd.Start()
	if err != nil {
		return err
	}
	err = setKitePid(folder, cmd.Process.Pid)
	if err != nil {
		return err
	}
	return nil
}

func kiteExists(folder string) bool {
	inf, err := os.Stat(folder)
	if err != nil {
		return false
	}
	return inf.IsDir()
}

func kiteRunning(folder string) (error, bool) {
	pid, err := getKitePid(folder)
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

func killKite(folder string) error {
	pid, err := getKitePid(folder)
	if err != nil {
		return err
	}
	if pid != 0 {
		err := process.KillPid(pid)
		if err != nil {
			return err
		}
	}
	return nil
}

/****************************************

kd stop

****************************************/

type Stop struct{}

func NewStop() *Stop {
	return &Stop{}
}

func (s Stop) Definition() string {
	return "Stops a running kite"
}

func (s Stop) Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a kite name")
	}

	kiteName := flag.Arg(0)
	folder := kiteName + ".kite"
	if !kiteExists(folder) {
		return fmt.Errorf("There is no kite named %s", kiteName)
	}
	err, ok := kiteRunning(folder)
	if err != nil {
		return nil
	}
	if !ok {
		return fmt.Errorf("Kite is not running")
	}
	err = killKite(folder)
	if err != nil {
		return err
	}
	fmt.Println("Stopped kite")
	err = setKitePid(folder, 0)
	if err != nil {
		return err
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

func (c Create) Definition() string {
	return "Creates a kite from kite skeleton"
}

func (c Create) Exec() error {
	flag.Parse()
	if len(flag.Args()) == 0 {
		return errors.New("You should give a kite name")
	}

	kiteName := flag.Arg(0)
	folder := kiteName + ".kite"

	err := os.Mkdir(folder, 0755)
	if os.IsExist(err) {
		return errors.New("Kite already exists")
	}
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(folder, "bin"), 0755)
	if err != nil {
		return err
	}
	err = os.MkdirAll(filepath.Join(folder, "resources"), 0755)
	if err != nil {
		return err
	}
	err = createManifest(folder, kiteName)
	if err != nil {
		return err
	}
	err = createKite(folder, kiteName)
	if err != nil {
		return err
	}
	return nil
}

func createManifest(folder, kiteName string) error {
	path := filepath.Join(folder, "manifest.json")
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	options := &protocol.Options{
		Username:     currUser.Name,
		Kitename:     kiteName,
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

func createKite(folder, kiteName string) error {
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	skelPath := filepath.Join(currUser.HomeDir, "/.kd/skel.go")
	kitePath := filepath.Join(folder, kiteName+".go")
	return cp(skelPath, kitePath)
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
