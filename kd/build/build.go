package build

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"koding/kite/kd/util"
	"koding/kite/kd/util/deps"
	"log"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

type Build struct {
	AppName    string
	Version    string
	Output     string
	BinaryPath string
	ImportPath string
	Files      string

	// used for pkg packaging
	Identifier string
}

func NewBuild() *Build {
	return &Build{
		Version: "0.0.1",
	}
}

func (b *Build) Definition() string {
	return "Build deployable install packages"
}

func (b *Build) Exec(args []string) error {
	usage := "Usage: kd build --import <importPath> || --bin <binaryPath> --files <filesPath>"
	if len(args) == 0 {
		return errors.New(usage)
	}

	f := flag.NewFlagSet("build", flag.ContinueOnError)
	f.StringVar(&b.ImportPath, "import", "", "Go importpath to be packaged")
	f.StringVar(&b.BinaryPath, "bin", "", "Binary to be packaged")
	f.StringVar(&b.Files, "files", "", "Files to be included with the package")
	f.StringVar(&b.Identifier, "identifier", "com.koding", "Pkg identifier")
	f.Parse(args)

	err := b.InitializeAppName()
	if err != nil {
		return err
	}

	b.Version = "0.0.1"
	b.Output = fmt.Sprintf("%s-%s.%s-%s", b.AppName, b.Version, runtime.GOOS, runtime.GOARCH)

	err = b.Do()
	if err != nil {
		return err
	}

	return nil
}

func (b *Build) InitializeAppName() error {
	if b.BinaryPath != "" {
		b.AppName = filepath.Base(b.BinaryPath)
	} else if b.ImportPath != "" {
		b.AppName = filepath.Base(b.ImportPath)
	} else {
		return errors.New("build: --import or --bin should be defined.")
	}

	return nil
}

func (b *Build) Do() error {
	switch runtime.GOOS {
	case "darwin":
		err := b.Darwin()
		if err != nil {
			log.Println("darwin:", err)
		}
	case "linux":
		err := b.Linux()
		if err != nil {
			log.Println("linux:", err)
		}
	}

	// also create a tar.gz regardless of os
	return b.TarGzFile()
}

func (b *Build) TarGzFile() error {
	buildFolder, err := ioutil.TempDir(".", "kd-build")
	if err != nil {
		return err
	}
	defer os.RemoveAll(buildFolder)

	if b.ImportPath != "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return errors.New("GOPATH is not set")
		}

		// or use "go list <importPath>" for all packages and commands
		packages := []string{b.ImportPath}
		d, err := deps.LoadDeps(packages...)
		if err != nil {
			return err
		}

		err = d.InstallDeps()
		if err != nil {
			return err
		}

		buildFolder = filepath.Join(d.BuildGoPath, b.AppName)
		defer os.RemoveAll(d.BuildGoPath)

	} else {
		err := util.Copy(b.BinaryPath, buildFolder)
		if err != nil {
			log.Println("copy binaryPath", err)
		}
	}

	// include given files
	if b.Files != "" {
		files := strings.Split(b.Files, ",")
		for _, path := range files {
			err := util.Copy(path, buildFolder)
			if err != nil {
				log.Println("copy assets", err)
			}
		}
	}

	// create tar.gz file from final director
	tarFile := b.Output + ".tar.gz"
	err = util.MakeTar(tarFile, filepath.Dir(buildFolder))
	if err != nil {
		return err
	}

	fmt.Printf("'%s' is created and ready for deploy\n", tarFile)
	return nil
}
