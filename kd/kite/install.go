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

const S3URL = "http://koding-kites.s3.amazonaws.com/"

func (*Install) Exec() error {
	// Parse kite name
	flag.Parse()
	if flag.NArg() != 1 {
		return errors.New("You should give a kite name")
	}
	kiteFullName := flag.Arg(0)
	kiteName, kiteVersion, err := splitVersion(kiteFullName, true)
	if err != nil {
		kiteName, kiteVersion = kiteFullName, "latest"
	}

	// Make download request
	fmt.Println("Downloading...")
	targz, err := requestPackage(kiteName, kiteVersion)
	if err != nil {
		return err
	}
	defer targz.Close()

	// Extract gzip
	gz, err := gzip.NewReader(targz)
	if err != nil {
		return err
	}
	defer gz.Close()

	// Extract tar
	tempKitePath, err := ioutil.TempDir("", "kd-kite-install-")
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
	kiteFullName, err = moveFromTempToHome(kiteName, tempKitePath)
	if err != nil {
		return err
	}

	fmt.Println("Installed successfully:", kiteFullName)
	return nil
}

// requestPackage makes a request to the kite repository and returns
// a io.ReadCloser. The caller must close the returned io.ReadCloser.
func requestPackage(kiteName, kiteVersion string) (io.ReadCloser, error) {
	kiteURL := S3URL + kiteName + "-" + kiteVersion + ".kite.tar.gz"
	log.Println(kiteURL)

	res, err := http.Get(kiteURL)
	if err != nil {
		return nil, err
	}
	if res.StatusCode == 404 {
		res.Body.Close()
		return nil, errors.New("Package is not found on the server.")
	}
	if res.StatusCode != 200 {
		res.Body.Close()
		return nil, fmt.Errorf("Unexpected response from server: %d", res.StatusCode)
	}
	return res.Body, nil
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

// moveFromTempToHome make some assertions about the bundle extracted from
// package, then it moves the .kite bundle into ~/kd/kites.
// Returns the full kite name moved.
func moveFromTempToHome(kiteName, tempKitePath string) (string, error) {
	dirs, err := ioutil.ReadDir(tempKitePath)
	if err != nil {
		return "", err
	}
	if len(dirs) != 1 {
		return "", errors.New("Invalid package: Package must contain only one directory.")
	}
	// found prefix means we got it from extracted tar.
	// We should assert that they are expected.
	foundKiteBundleName := dirs[0].Name() // Example: asdf-1.2.3.kite
	if !strings.HasSuffix(foundKiteBundleName, ".kite") {
		return "", errors.New("Invalid package: Direcory name must end with \".kite\".")
	}
	foundKiteFullName := strings.TrimSuffix(foundKiteBundleName, ".kite") // Example: asdf-1.2.3
	foundKiteName, _, err := splitVersion(foundKiteFullName, false)
	if err != nil {
		return "", errors.New("Invalid package: No version number in Kite bundle")
	}
	if foundKiteName != kiteName {
		return "", fmt.Errorf("Invalid package: Bundle name does not match with package name: %s != %s", foundKiteName, kiteName)
	}
	tempKitePath = filepath.Join(tempKitePath, foundKiteBundleName)
	kitesPath := filepath.Join(util.GetKdPath(), "kites")
	os.MkdirAll(kitesPath, 0700)
	kitePath := filepath.Join(kitesPath, foundKiteBundleName)
	log.Println("Moving from:", tempKitePath, "to:", kitePath)
	err = os.Rename(tempKitePath, kitePath)
	if err != nil {
		return "", err
	}
	return foundKiteFullName, nil
}

// splitVersion takes a name like "asdf-1.2.3" and
// returns the name "asdf" and version "1.2.3" seperately.
// If allowLatest is true, then the version must not be numeric and can be "latest".
func splitVersion(fullname string, allowLatest bool) (name, version string, err error) {
	notFound := errors.New("name does not contain a version number")

	parts := strings.Split(fullname, "-")
	n := len(parts)
	if n < 2 {
		return "", "", notFound
	}

	name = strings.Join(parts[:n-1], "-")
	version = parts[n-1]

	if allowLatest && version == "latest" {
		return name, version, nil
	}

	versionParts := strings.Split(version, ".")
	for _, v := range versionParts {
		if _, err := strconv.ParseUint(v, 10, 64); err != nil {
			return "", "", notFound
		}
	}

	return name, version, nil
}
