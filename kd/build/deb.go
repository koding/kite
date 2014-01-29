package build

import (
	"errors"
	"fmt"
	"go/build"
	"koding/kite/kd/util"
	"koding/kite/kd/util/deps"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/template"
)

type Deb struct {
	// App informations
	AppName string
	Version string
	Desc    string
	Arch    string

	// Build fields
	Output          string
	ImportPath      string
	InstallPrefix   string
	BuildFolder     string
	Files           string
	UpstartScript   string
	DebianTemplates map[string]string
}

// Deb is building a new .deb package with the provided tarFile It returns the
// created filename of the .deb file.
func (d *Deb) Build() (string, error) {
	defer d.cleanDebianBuild()

	d.BuildFolder = deps.DepsGoPath
	d.Arch = debArch()
	d.Desc = d.AppName + " Kite"
	d.DebianTemplates = d.debianTemplates()
	d.Output = fmt.Sprintf("%s_%s_%s", d.AppName, d.Version, d.Arch)

	if err := d.createDebianDir(); err != nil {
		return "", err
	}

	if err := d.createInstallDir(); err != nil {
		return "", err
	}

	// finally build with debuild to create .deb file
	cmd := exec.Command("debuild", "-us", "-uc")
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Dir = d.BuildFolder

	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return d.Output, nil
}

func (d *Deb) cleanDebianBuild() {
	os.RemoveAll(d.BuildFolder)

	exts := []string{"build", "changes"}
	output := fmt.Sprintf("%s_%s_%s", d.AppName, d.Version, d.Arch)
	for _, ext := range exts {
		os.Remove(output + "." + ext)
	}

	exts = []string{"dsc", "tar.gz"}
	output = fmt.Sprintf("%s_%s", d.AppName, d.Version)
	for _, ext := range exts {
		os.Remove(output + "." + ext)
	}
}

func (d *Deb) createInstallDir() error {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return errors.New("GOPATH is not set")
	}

	// or use "go list <importPath>" for all packages and commands
	packages := []string{d.ImportPath}
	dp, err := deps.LoadDeps(packages...)
	if err != nil {
		return err
	}

	err = dp.InstallDeps()
	if err != nil {
		return err
	}

	appFolder := filepath.Join(dp.BuildGoPath, d.AppName)

	if d.Files != "" {
		files := strings.Split(d.Files, ",")
		for _, path := range files {
			err := util.Copy(path, appFolder)
			if err != nil {
				log.Println("copy assets", err)
			}
		}
	}

	if d.UpstartScript != "" {
		upstartPath := filepath.Join(d.BuildFolder, "debian/")
		upstartFile := filepath.Base(d.UpstartScript)

		err := util.Copy(d.UpstartScript, upstartPath)
		if err != nil {
			log.Println("copy assets", err)
		}

		oldFile := filepath.Join(upstartPath, upstartFile)
		newFile := filepath.Join(upstartPath, d.AppName+".upstart")

		err = os.Rename(oldFile, newFile)
		if err != nil {
			return err
		}
	}

	// move files to installprefix
	os.MkdirAll(filepath.Join(d.BuildFolder, d.InstallPrefix), 0755)
	installFolder := filepath.Join(d.BuildFolder, d.InstallPrefix, d.AppName)
	if err := os.Rename(appFolder, installFolder); err != nil {
		return err
	}

	return nil
}

func (d *Deb) createDebianDir() error {
	debianFolder := filepath.Join(d.BuildFolder, "debian")
	os.MkdirAll(debianFolder, 0755)

	if err := d.createDebianFile("control"); err != nil {
		return err
	}

	if err := d.createDebianFile("rules"); err != nil {
		return err
	}

	// make debian/rules executable
	os.Chmod(filepath.Join(debianFolder, "rules"), 0755)

	if err := d.createDebianFile("compat"); err != nil {
		return err
	}

	if err := d.createDebianFile("install"); err != nil {
		return err
	}

	if err := d.createDebianFile("changelog"); err != nil {
		return err
	}

	return nil
}

func (d *Deb) createDebianFile(name string) error {
	debianFile := filepath.Join(d.BuildFolder, "debian", name)
	file, err := os.Create(debianFile)
	if err != nil {
		return err
	}
	defer file.Close()

	return template.
		Must(template.New("controlFile").
		Parse(d.DebianTemplates[name])).
		Execute(file, d)
}

func (d *Deb) debianTemplates() map[string]string {
	t := make(map[string]string)
	t["control"] = `Source: {{.AppName}}
Section: devel
Priority: extra
Standards-Version: {{.Version}}
Maintainer: Koding Developers <hello@koding.com>
Homepage: https://koding.com

Package: {{.AppName}}
Architecture: {{.Arch}}
Description: {{.Desc}}
`

	t["rules"] = `#!/usr/bin/make -f
%:
	dh $@
`

	t["changelog"] = `{{.AppName}} ({{.Version}}) raring; urgency=low

  * Initial release.

 -- Koding Developers <hello@koding.com>  Tue, 28 Jan 2014 22:17:54 -0800
`

	t["compat"] = "9"

	t["install"] = fmt.Sprintf("%s/ /", filepath.Dir(d.InstallPrefix))

	return t
}

func debArch() string {
	arch := build.Default.GOARCH
	if arch == "386" {
		return "i386"
	}
	return arch
}
