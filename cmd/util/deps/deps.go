package deps

import (
	"encoding/json"
	"errors"
	"fmt"
	"go/build"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"sort"

	"github.com/fatih/set"
)

const (
	DepsGoPath    = "gopackage"
	gopackageFile = "gopackage.json"
)

// Pkg defines a single go package entity
type Pkg struct {
	ImportPath string `json:"importPath"`
	Output     string `json:"output"`
}

// NewPkg returns a new Pkg struct. If output is empty, t
func NewPkg(importPath, output string) Pkg {
	return Pkg{
		ImportPath: importPath,
		Output:     output,
	}
}

type Deps struct {
	// Packages is written as the importPath of a given package(s).
	Packages []Pkg `json:"packages"`

	// GoVersion defines the Go version needed at least as a minumum.
	GoVersion string `json:"goVersion"`

	// Dependencies defines the dependency of the given Packages. If multiple
	// packages are defined, each dependency will point to the HEAD unless
	// changed manually.
	Dependencies []string `json:"dependencies"`

	// BuildGoPath is used to fetch dependencies of the given Packages
	BuildGoPath string

	// currentGoPath, is taken from current GOPATH environment variable
	currentGoPath string
}

// LoadDeps returns a new Deps struct with the given packages. It founds the
// dependencies and populates the fields in Deps. After LoadDeps one can use
// InstallDeps() to install/build the binary for the given pkg or use
// GetDeps() to download and vendorize the dependencies of the given pkg.
func LoadDeps(pkgs ...Pkg) (*Deps, error) {
	packages, err := listPackages(pkgs...)
	if err != nil {
		fmt.Println(err)
	}

	// get all dependencies for applications defined above
	dependencies := set.New()
	for _, pkg := range packages {
		for _, imp := range pkg.Deps {
			dependencies.Add(imp)
		}
	}

	// clean up deps
	// 1. remove std lib paths
	context := build.Default
	thirdPartyDeps := make([]string, 0)

	for _, importPath := range set.StringSlice(dependencies) {
		p, err := context.Import(importPath, ".", build.AllowBinary)
		if err != nil {
			log.Println(err)
		}

		// do not include std lib
		if p.Goroot {
			continue
		}

		thirdPartyDeps = append(thirdPartyDeps, importPath)
	}

	sort.Strings(thirdPartyDeps)

	deps := &Deps{
		Packages:     pkgs,
		Dependencies: thirdPartyDeps,
		GoVersion:    runtime.Version(),
	}

	err = deps.populateGoPaths()
	if err != nil {
		return nil, err
	}

	return deps, nil
}

func (d *Deps) populateGoPaths() error {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return errors.New("GOPATH is not set")
	}

	pwd, err := os.Getwd()
	if err != nil {
		return err
	}

	d.currentGoPath = gopath
	d.BuildGoPath = path.Join(pwd, DepsGoPath)
	return nil
}

// InstallDeps calls "go build" on the given packages and installs them
// to deps.BuildGoPath/pkgname
func (d *Deps) InstallDeps() error {
	if !compareGoVersions(d.GoVersion, runtime.Version()) {
		return fmt.Errorf("Go Version is not satisfied\nSystem Go Version: '%s' Expected: '%s'",
			runtime.Version(), d.GoVersion)
	}

	// another approach is let them building with a single gobin and then move
	// the final binaries into new directories based on the binary filename.
	for _, pkg := range d.Packages {
		var binPath string
		if pkg.Output == "" {
			binPath = filepath.Join(d.BuildGoPath, filepath.Base(pkg.ImportPath))
		} else {
			binPath = filepath.Join(d.BuildGoPath, pkg.Output)
		}
		os.MkdirAll(binPath, 0755)

		binFile := filepath.Join(binPath, filepath.Base(binPath))

		args := []string{"build", "-o", binFile, pkg.ImportPath}
		cmd := exec.Command("go", args...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

		err := cmd.Run()
		if err != nil {
			log.Println(err)
		}
	}

	return nil
}

func goPackagePath() string {
	pwd, err := os.Getwd()
	if err != nil {
		return ""
	}

	return path.Join(pwd, gopackageFile)
}

// WriteJSON writes the state of d into a json file, useful to restore again
// with ReadJSON().
func (d *Deps) WriteJSON() error {
	data, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}

	err = ioutil.WriteFile(goPackagePath(), data, 0755)
	if err != nil {
		return err
	}

	return nil
}

// ReadJSON() restores a given gopackage.json to create a new deps struct.
func ReadJson() (*Deps, error) {
	data, err := ioutil.ReadFile(goPackagePath())
	if err != nil {
		return nil, err
	}

	d := new(Deps)
	err = json.Unmarshal(data, d)
	if err != nil {
		return nil, err
	}

	err = d.populateGoPaths()
	if err != nil {
		return nil, err
	}

	return d, nil
}

// GetDeps calls "go get -d" to download all dependencies for the packages
// defined in d.
func (d *Deps) GetDeps() error {
	os.MkdirAll(d.BuildGoPath, 0755)
	os.Setenv("GOPATH", d.BuildGoPath)

	for _, pkg := range d.Dependencies {
		fmt.Println("go get", pkg)
		cmd := exec.Command("go", []string{"get", "-d", pkg}...)
		cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr

		err := cmd.Run()
		if err != nil {
			log.Println(err)
		}
	}

	return nil
}
