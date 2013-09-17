package main

import (
	"bytes"
	"errors"
	"flag"
	"fmt"
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
	Output dnode.Callback
}

type Webterm struct{}

var port = flag.String("port", "", "port to bind itself")

func main() {
	flag.Parse()
	o := &protocol.Options{Username: "fatih", Kitename: "os-local", Version: "1", Port: *port}
	k := kite.New(o, new(Webterm))
	k.Start()
}

func (Webterm) Info(r *protocol.KiteRequest, result *bool) error {
	*result = true
	return nil
}

func (Webterm) Connect(r *protocol.KiteRequest, result *WebtermServer) error {
	var params struct {
		Remote       WebtermRemote
		SizeX, SizeY int
		NoScreen     bool
	}

	if r.ArgsDnode.Unmarshal(&params) != nil || params.SizeX <= 0 || params.SizeY <= 0 {
		return errors.New("{ remote: [object], session: [string], sizeX: [integer], sizeY: [integer], noScreen: [boolean] }")
	}

	fmt.Printf("Connect details %#v\n", params)
	server := &WebtermServer{
		remote: params.Remote,
		pty:    pty.New(),
	}

	server.SetSize(float64(params.SizeX), float64(params.SizeY))
	fmt.Println("params size x and y", params.SizeX, params.SizeY)

	c := exec.Command("/usr/bin/screen", "-e^Bb", "-S", "koding")
	// c := exec.Command("/bin/zsh")
	c.Stdout = server.pty.Slave
	c.Stdin = server.pty.Slave
	c.Stderr = server.pty.Slave
	// c.SysProcAttr = &syscall.SysProcAttr{Setctty: true, Setsid: true}

	err := c.Start()
	if err != nil {
		log.Println("could not start", err)
	}

	// go func() {
	// 	server.pty.Slave.Close()
	// 	server.pty.Master.Close()
	// }()

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
