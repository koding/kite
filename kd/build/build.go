package build

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"path/filepath"
	"runtime"
)

type Build struct{}

func NewBuild() *Build {
	return &Build{}
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

	version := f.String("version", "0.0.1", "Version of the package")
	binaryPath := f.String("bin", "", "Binary to be packaged")
	importPath := f.String("import", "", "Go importpath to be packaged")
	files := f.String("files", "", "Files to be included with the package")
	identifier := f.String("identifier", "com.koding", "Pkg identifier")

	f.Parse(args)

	var (
		appName string
		pkgFile string
		err     error
	)

	if *binaryPath != "" {
		appName = filepath.Base(*binaryPath)
	} else if *importPath != "" {
		appName = filepath.Base(*importPath)
	} else {
		return errors.New("build: --import or --bin should be defined.")
	}

	output := fmt.Sprintf("%s-%s.%s-%s",
		appName, *version, runtime.GOOS, runtime.GOARCH)

	switch runtime.GOOS {
	case "darwin":
		darwin := &Darwin{
			AppName:    appName,
			BinaryPath: *binaryPath,
			Version:    *version,
			Identifier: *identifier,
			Output:     output,
		}

		pkgFile, err = darwin.Build()
		if err != nil {
			log.Println("darwin:", err)
		}
	case "linux":
		tar := &Tar{
			AppName:    appName,
			Files:      *files,
			ImportPath: *importPath,
			BinaryPath: *binaryPath,
			Output:     output,
		}

		tarFile, err := tar.Build()
		if err != nil {
			return err
		}

		deb := &Deb{
			AppName:       appName,
			Version:       *version,
			Output:        output,
			TarFile:       tarFile,
			InstallPrefix: "/opt/kite",
		}

		pkgFile, err = deb.Build()
		if err != nil {
			log.Println("linux:", err)
		}
	}

	fmt.Println("package  :", pkgFile, "ready")
	return nil
}
