// Package util contains the shared functions and constants for cli package.
package util

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/nu7hatch/gouuid"
)

var KeyPath = filepath.Join(GetKdPath(), "koding.key")

const (
	AuthServer      = "https://koding.com"
	AuthServerLocal = "http://localhost:3020"
)

// getKdPath returns absolute of ~/.kd
func GetKdPath() string {
	usr, err := user.Current()
	if err != nil {
		panic(err)
	}

	return filepath.Join(usr.HomeDir, ".kd")
}

// GetKey returns the Koding key content from ~/.kd/koding.key
func GetKey() (string, error) {
	data, err := ioutil.ReadFile(KeyPath)
	if err != nil {
		return "", err
	}

	key := strings.TrimSpace(string(data))

	return key, nil
}

// WriteKey writes the content of the given key to ~/.kd/koding.key
func WriteKey(key string) error {
	os.Mkdir(GetKdPath(), 0700) // create if not exists

	err := ioutil.WriteFile(KeyPath, []byte(key), 0600)
	if err != nil {
		return err
	}

	return nil
}

// CheckKey checks wether the key is registerd to koding, or not
func CheckKey(authServer, key string) error {
	checkUrl := CheckURL(authServer, key)

	resp, err := http.Get(checkUrl)
	if err != nil {
		return err
	}

	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	resp.Body.Close()

	if resp.StatusCode != 200 {
		return errors.New("non 200 response")
	}

	type Result struct {
		Result string `json:"result"`
	}

	res := Result{}
	err = json.Unmarshal(bytes.TrimSpace(body), &res)
	if err != nil {
		log.Fatalln(err) // this should not happen, exit here
	}

	return nil
}

func CheckURL(authServer, key string) string {
	return fmt.Sprintf("%s/-/auth/check/%s", authServer, key)
}

// hostID returns a unique string that defines a machine
func HostID() (string, error) {
	id, err := uuid.NewV4()
	if err != nil {
		return "", err
	}

	hostname, err := os.Hostname()
	if err != nil {
		return "", err
	}

	return hostname + "-" + id.String(), nil
}

// got it from http://golang.org/misc/dist/bindist.go?m=text and removed go
// related stuff, works perfect. It creates a tar.gz container from the given
// workdir.
func MakeTar(targ, workdir string) error {
	f, err := os.Create(targ)
	if err != nil {
		return err
	}
	zout := gzip.NewWriter(f)
	tw := tar.NewWriter(zout)

	err = filepath.Walk(workdir, func(path string, fi os.FileInfo, err error) error {
		if !strings.HasPrefix(path, workdir) {
			log.Panicf("walked filename %q doesn't begin with workdir %q", path, workdir)
		}
		name := path[len(workdir):]

		// Chop of any leading / from filename, leftover from removing workdir.
		if strings.HasPrefix(name, "/") {
			name = name[1:]
		}

		log.Printf("adding to tar: %s", name)

		target, _ := os.Readlink(path)
		hdr, err := tar.FileInfoHeader(fi, target)
		if err != nil {
			return err
		}

		hdr.Name = name
		hdr.Uname = "root"
		hdr.Gname = "root"
		hdr.Uid = 0
		hdr.Gid = 0

		// Force permissions to 0755 for executables, 0644 for everything else.
		if fi.Mode().Perm()&0111 != 0 {
			hdr.Mode = hdr.Mode&^0777 | 0755
		} else {
			hdr.Mode = hdr.Mode&^0777 | 0644
		}

		err = tw.WriteHeader(hdr)
		if err != nil {
			return fmt.Errorf("Error writing file %q: %v", name, err)
		}

		if fi.IsDir() {
			return nil
		}

		r, err := os.Open(path)
		if err != nil {
			return err
		}
		defer r.Close()

		_, err = io.Copy(tw, r)
		return err
	})

	if err != nil {
		return err
	}

	if err := tw.Close(); err != nil {
		return err
	}

	if err := zout.Close(); err != nil {
		return err
	}

	return f.Close()
}

func CopyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return errors.New("src is a directory, please provide a file")
	}

	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer df.Close()

	if _, err := io.Copy(df, sf); err != nil {
		return err
	}

	return nil
}
