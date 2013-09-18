package main

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"flag"
	"koding/newkite/kite"
	"koding/newkite/protocol"
	"koding/tools/dnode"
	"koding/tools/pty"
	"log"
	"os/exec"
	"syscall"
	"time"
	"unicode/utf8"
)

type WebtermServer struct {
	Session          string `json:"session"`
	remote           WebtermRemote
	isForeignSession bool
	pty              *pty.PTY
	currentSecond    int64
	messageCounter   int
	byteCounter      int
	lineFeeedCounter int
}

type WebtermRemote struct {
	Output       dnode.Callback
	SessionEnded dnode.Callback
}

type Terminal struct{}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	o := &protocol.Options{Username: "fatih", Kitename: "terminal-local", Version: "1", Port: *port}

	methods := map[string]interface{}{
		"vm.info":         Terminal.Info,
		"webterm.connect": Terminal.Connect,
	}

	k := kite.New(o, new(Terminal), methods)
	k.Start()
}

func (Terminal) Info(r *protocol.KiteDnodeRequest, result *bool) error {
	*result = true
	return nil
}

func (Terminal) Connect(r *protocol.KiteDnodeRequest, result *WebtermServer) error {
	var params struct {
		Remote       WebtermRemote
		Session      string
		SizeX, SizeY int
		NoScreen     bool
	}

	if r.Args.Unmarshal(&params) != nil || params.SizeX <= 0 || params.SizeY <= 0 {
		return errors.New("{ remote: [object], session: [string], sizeX: [integer], sizeY: [integer], noScreen: [boolean] }")
	}

	if params.NoScreen && params.Session != "" {
		return errors.New("The 'noScreen' and 'session' parameters can not be used together.")
	}

	newSession := false
	if params.Session == "" {
		params.Session = RandomString()
		newSession = true
	}

	server := &WebtermServer{
		Session: params.Session,
		remote:  params.Remote,
		pty:     pty.New("/dev"),
	}

	server.SetSize(float64(params.SizeX), float64(params.SizeY))

	var command struct {
		name string
		args []string
	}

	command.name = "/usr/bin/screen"
	command.args = []string{"-e^Bb", "-S", "koding." + params.Session}
	// tmux version, attach to an existing one, if not available it creates one
	// command.name = "/usr/local/bin/tmux"
	// command.args = []string{"tmux", "attach", "-t", "koding." + params.Session, "||", "tmux", "new-session", "-s", "koding." + params.Session}

	if !newSession {
		command.args = append(command.args, "-x")
	}

	if params.NoScreen {
		command.name = "/bin/bash"
		command.args = []string{}
	}

	cmd := exec.Command(command.name, command.args...)
	cmd.Stdin = server.pty.Slave
	// cmd.Stdout = server.pty.Slave
	// cmd.Stderr = server.pty.Slave

	// Open in background
	// cmd.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}

	err := cmd.Start()
	if err != nil {
		log.Println("could not start", err)
	}

	go func() {
		cmd.Wait()
		server.pty.Slave.Close()
		server.pty.Master.Close()
		server.remote.SessionEnded()
	}()

	go func() {
		buf := make([]byte, (1<<12)-utf8.UTFMax, 1<<12)
		for {
			n, err := server.pty.Master.Read(buf)
			for n < cap(buf)-1 {
				r, _ := utf8.DecodeLastRune(buf[:n])
				if r != utf8.RuneError {
					break
				}
				server.pty.Master.Read(buf[n : n+1])
				n++
			}

			s := time.Now().Unix()
			if server.currentSecond != s {
				server.currentSecond = s
				server.messageCounter = 0
				server.byteCounter = 0
				server.lineFeeedCounter = 0
			}
			server.messageCounter += 1
			server.byteCounter += n
			server.lineFeeedCounter += bytes.Count(buf[:n], []byte{'\n'})
			if server.messageCounter > 100 || server.byteCounter > 1<<18 || server.lineFeeedCounter > 300 {
				time.Sleep(time.Second)
			}

			server.remote.Output(string(FilterInvalidUTF8(buf[:n])))
			if err != nil {
				break
			}
		}
	}()

	*result = *server
	return nil
}

func (server *WebtermServer) Input(data string) {
	server.pty.Master.Write([]byte(data))
}

func (server *WebtermServer) ControlSequence(data string) {
	server.pty.MasterEncoded.Write([]byte(data))
}

func (server *WebtermServer) SetSize(x, y float64) {
	server.pty.SetSize(uint16(x), uint16(y))
}

func (server *WebtermServer) Close() error {
	server.pty.Signal(syscall.SIGHUP)
	return nil
}

func (server *WebtermServer) Terminate() error {
	return server.Close()
}

func FilterInvalidUTF8(buf []byte) []byte {
	i := 0
	j := 0
	for {
		r, l := utf8.DecodeRune(buf[i:])
		if l == 0 {
			break
		}
		if r < 0xD800 {
			if i != j {
				copy(buf[j:], buf[i:i+l])
			}
			j += l
		}
		i += l
	}
	return buf[:j]
}

const RandomStringLength = 24 // 144 bit base64 encoded

func RandomString() string {
	r := make([]byte, RandomStringLength*6/8)
	rand.Read(r)
	return base64.URLEncoding.EncodeToString(r)
}
