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
	identifier := f.String("identifier", "com.koding", "Pkg identifier")

	f.Parse(args)

	err := b.InitializeAppName()
	if err != nil {
		return err
	}

	b.Version = "0.0.1"
	b.Output = fmt.Sprintf("%s-%s.%s-%s", b.AppName, b.Version, runtime.GOOS, runtime.GOARCH)

	var pkgFile string

	switch runtime.GOOS {
	case "darwin":
		d := &Darwin{
			AppName:    b.AppName,
			BinaryPath: b.BinaryPath,
			Version:    b.Version,
			Identifier: *identifier,
			Output:     b.Output,
		}

		pkgFile, err = d.Build()
		if err != nil {
			log.Println("darwin:", err)
		}
	case "linux":
		pkgFile, err = b.Linux()
		if err != nil {
			log.Println("linux:", err)
		}
	}

	// also create a tar.gz regardless of GOOS
	tarFile, err := b.TarGzFile()
	if err != nil {
		return err
	}

	fmt.Println("package  :", pkgFile, "ready")
	fmt.Println("tar file :", tarFile, "ready")

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

// TarGzFile creates and returns the filename of the created file
func (b *Build) TarGzFile() (string, error) {

	var buildFolder string

	if b.ImportPath != "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return "", errors.New("GOPATH is not set")
		}

		// or use "go list <importPath>" for all packages and commands
		packages := []string{b.ImportPath}
		d, err := deps.LoadDeps(packages...)
		if err != nil {
			return "", err
		}

		err = d.InstallDeps()
		if err != nil {
			return "", err
		}

		buildFolder = filepath.Join(d.BuildGoPath, b.AppName)
		defer os.RemoveAll(d.BuildGoPath)

	} else {
		tmpDir, err := ioutil.TempDir(".", "kd-build")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmpDir)

		buildFolder = filepath.Join(tmpDir, b.AppName)
		os.MkdirAll(buildFolder, 0755)

		err = util.Copy(b.BinaryPath, buildFolder)
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
	err := util.MakeTar(tarFile, filepath.Dir(buildFolder))
	if err != nil {
		return "", err
	}

	return tarFile, nil
}
