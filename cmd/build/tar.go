package build

import (
	"errors"
	"io/ioutil"
	"github.com/koding/kite/cmd/util"
	"github.com/koding/kite/cmd/util/deps"
	"log"
	"os"
	"path/filepath"
	"strings"
)

type Tar struct {
	AppName    string
	Files      string
	ImportPath string
	BinaryPath string
	Output     string
}

// TarGzFile creates and returns the filename of the created file
func (t *Tar) Build() (string, error) {
	var buildFolder string

	if t.ImportPath != "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return "", errors.New("GOPATH is not set")
		}

		// or use "go list <importPath>" for all packages and commands
		d, err := deps.LoadDeps(deps.NewPkg(t.ImportPath, t.AppName))
		if err != nil {
			return "", err
		}

		err = d.InstallDeps()
		if err != nil {
			return "", err
		}

		buildFolder = filepath.Join(d.BuildGoPath, t.AppName)
		defer os.RemoveAll(d.BuildGoPath)

	} else {
		tmpDir, err := ioutil.TempDir(".", "kd-build")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		buildFolder = filepath.Join(tmpDir, t.AppName)
		os.MkdirAll(buildFolder, 0755)

		err = util.Copy(t.BinaryPath, buildFolder)
		if err != nil {
			log.Println("copy binaryPath", err)
		}
	}

	// include given files
	if t.Files != "" {
		files := strings.Split(t.Files, ",")
		for _, path := range files {
			err := util.Copy(path, buildFolder)
			if err != nil {
				log.Println("copy assets", err)
			}
		}
	}

	// create tar.gz file from final director
	tarFile := t.Output + ".tar.gz"
	err := util.MakeTar(tarFile, filepath.Dir(buildFolder))
	if err != nil {
		return "", err
	}

	return tarFile, nil
}
