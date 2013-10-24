package kite

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"koding/newKite/kd/util"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

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
	kitesPath := filepath.Join(util.GetKdPath(), "kites")
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

// splitVersion takes a name like "asdf-1.2.3" and
// returns the name "asdf" and version "1.2.3" seperately.
func splitVersion(fullname string) (name, version string, err error) {
	notFound := errors.New("name does not contain a version number")

	parts := strings.Split(fullname, "-")
	n := len(parts)
	if n < 2 {
		return "", "", notFound
	}

	version = parts[n-1]
	versionParts := strings.Split(version, ".")
	for _, v := range versionParts {
		if _, err := strconv.ParseUint(v, 10, 64); err != nil {
			return "", "", notFound
		}
	}

	name = strings.Join(parts[:n-2], "-")
	return name, version, nil
}
