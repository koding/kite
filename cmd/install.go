package cmd

import (
	"archive/tar"
	"compress/gzip"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"github.com/koding/kite/kitekey"
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
	return "Install a kite from repository"
}

const S3URL = "http://koding-kites.s3.amazonaws.com/"

func (*Install) Exec(args []string) error {
	// Parse kite name
	if len(args) != 1 {
		return errors.New("You should give a kite name")
	}

	kiteFullName := args[0]
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
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempKitePath)

	err = extractTar(gz, tempKitePath)
	if err != nil {
		return err
	}

	foundName, foundVersion, bundlePath, err := validatePackage(tempKitePath)
	if err != nil {
		return err
	}

	if foundName != kiteName {
		return fmt.Errorf("Invalid package: Bundle name does not match with package name: %s != %s",
			foundName, kiteName)
	}

	err = installBundle(bundlePath)
	if err != nil {
		return err
	}

	fmt.Println("Installed successfully:", foundName+"-"+foundVersion)
	return nil
}

// requestPackage makes a request to the kite repository and returns
// a io.ReadCloser. The caller must close the returned io.ReadCloser.
func requestPackage(kiteName, kiteVersion string) (io.ReadCloser, error) {
	kiteURL := S3URL + kiteName + "-" + kiteVersion + ".kite.tar.gz"

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
	first := true // true if we are on the first entry of tarball
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

		// Check if the same kite version is installed before
		if first {
			first = false
			kiteName := strings.TrimSuffix(hdr.Name, ".kite/")

			installed, err := isInstalled(kiteName)
			if err != nil {
				return err
			}

			if installed {
				return fmt.Errorf("Already installed: %s", kiteName)
			}
		}

		path := filepath.Join(dir, hdr.Name)

		if hdr.FileInfo().IsDir() {
			os.MkdirAll(path, 0700)
		} else {
			mode := 0600
			if isBinaryFile(hdr.Name) {
				mode = 0700
			}

			f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, os.FileMode(mode))
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

// validatePackage returns the package name, version and bundle path.
func validatePackage(tempKitePath string) (string, string, string, error) {
	dirs, err := ioutil.ReadDir(tempKitePath)
	if err != nil {
		return "", "", "", err
	}

	if len(dirs) != 1 {
		return "", "", "", errors.New("Invalid package: Package must contain only one directory.")
	}

	bundleName := dirs[0].Name() // Example: asdf-1.2.3.kite
	if !strings.HasSuffix(bundleName, ".kite") {
		return "", "", "", errors.New("Invalid package: Direcory name must end with \".kite\".")
	}

	fullName := strings.TrimSuffix(bundleName, ".kite") // Example: asdf-1.2.3
	kiteName, version, err := splitVersion(fullName, false)
	if err != nil {
		return "", "", "", errors.New("Invalid package: No version number in Kite bundle")
	}

	return kiteName, version, filepath.Join(tempKitePath, bundleName), nil
}

// installBundle moves the .kite bundle into ~/kd/kites.
func installBundle(bundlePath string) error {
	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return err
	}

	kitesPath := filepath.Join(kiteHome, "kites")
	os.MkdirAll(kitesPath, 0700)

	bundleName := filepath.Base(bundlePath)
	kitePath := filepath.Join(kitesPath, bundleName)
	return os.Rename(bundlePath, kitePath)
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
		if _, err := strconv.Atoi(v); err != nil {
			return "", "", notFound
		}
	}

	return name, version, nil
}

// isBinaryFile returns true if the path is the path of the binary file
// in aplication bundle. Example: fs-0.0.1.kite/bin/fs
func isBinaryFile(path string) bool {
	parts := strings.Split(path, string(os.PathSeparator))
	if len(parts) != 3 {
		return false
	}

	binPath, err := getBinPath(parts[0])
	if err != nil {
		return false
	}

	return path == binPath
}

// getBinPath takes a bundle name and return the path of the kite executable.
// example: fs-0.0.1.kite -> fs-0.0.1/fs/bin
func getBinPath(bundleName string) (string, error) {
	if !strings.HasSuffix(bundleName, ".kite") {
		return "", fmt.Errorf("Invalid bundle name: %s", bundleName)
	}

	fullName := strings.TrimSuffix(bundleName, ".kite")
	name, _, err := splitVersion(fullName, false)
	if err != nil {
		return "", err
	}

	return strings.Join([]string{bundleName, "bin", name}, string(os.PathSeparator)), nil
}
