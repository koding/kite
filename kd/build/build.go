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
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"
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
	f.StringVar(&b.ImportPath, "import", "", "Go importpath to be packaged")
	f.StringVar(&b.BinaryPath, "bin", "", "Binary to be packaged")
	f.StringVar(&b.Files, "files", "", "Files to be included with the package")
	f.StringVar(&b.Identifier, "identifier", "com.koding", "Pkg identifier")
	f.Parse(args)

	if b.BinaryPath != "" {
		b.AppName = filepath.Base(b.BinaryPath)
	} else if b.ImportPath != "" {
		b.AppName = filepath.Base(b.ImportPath)
	} else {
		return errors.New("build: --import or --bin should be defined.")
	}

	b.Version = "0.0.1"
	b.Output = fmt.Sprintf("%s.%s-%s", b.AppName, runtime.GOOS, runtime.GOARCH)

	err := b.do()
	if err != nil {
		return err
	}

	return nil
}

func (b *Build) do() error {
	switch runtime.GOOS {
	case "darwin":
		err := b.darwin()
		if err != nil {
			log.Println("darwin:", err)
		}
	case "linux":
		err := b.linux()
		if err != nil {
			log.Println("linux:", err)
		}
	}

	// also create a tar.gz regardless of os
	return b.tarGzFile()
}

func (b *Build) linux() error {
	return nil
}

func (b *Build) tarGzFile() error {
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
	} else {
		err := util.Copy(b.BinaryPath, buildFolder)
		if err != nil {
			log.Println("copy assets", err)
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
	tarFile := fmt.Sprintf("%s.%s-%s.tar.gz", b.AppName, runtime.GOOS, runtime.GOARCH)
	err = util.MakeTar(tarFile, buildFolder)
	if err != nil {
		return err
	}

	fmt.Printf("'%s' is created and ready for deploy\n", tarFile)
	return nil
}

// darwin is building a new .pkg installer for darwin based OS'es.
func (b *Build) darwin() error {
	version := b.Version
	if b.Output == "" {
		b.Output = fmt.Sprintf("kite-%s", b.AppName)
	}

	installRoot, err := ioutil.TempDir(".", "kd-build-darwin_")
	if err != nil {
		return err
	}
	// defer os.RemoveAll(installRoot)

	buildFolder, err := ioutil.TempDir(".", "kd-build-darwin_")
	if err != nil {
		return err
	}
	// defer os.RemoveAll(buildFolder)

	scriptDir := filepath.Join(buildFolder, "scripts")
	installRootUsr := filepath.Join(installRoot, "/usr/local/bin")

	os.MkdirAll(installRootUsr, 0755)
	err = util.Copy(b.BinaryPath, installRootUsr+"/"+b.AppName)
	if err != nil {
		return err
	}

	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tempDest)

	b.createScripts(scriptDir)
	b.createLaunchAgent(installRoot)

	cmdPkg := exec.Command("pkgbuild",
		"--identifier", fmt.Sprintf("%s.kite.%s.pkg", b.Identifier, b.AppName),
		"--version", version,
		"--scripts", scriptDir,
		"--root", installRoot,
		"--install-location", "/",
		fmt.Sprintf("%s/%s.kite.%s.pkg", tempDest, b.Identifier, b.AppName),
		// used for next step, also set up for distribution.xml
	)

	_, err = cmdPkg.CombinedOutput()
	if err != nil {
		return err
	}

	distributionFile := filepath.Join(buildFolder, "Distribution.xml")
	resources := filepath.Join(buildFolder, "Resources")

	targetFile := b.Output + ".pkg"

	b.createDistribution(distributionFile)

	cmdBuild := exec.Command("productbuild",
		"--distribution", distributionFile,
		"--resources", resources,
		"--package-path", tempDest,
		targetFile,
	)

	_, err = cmdBuild.CombinedOutput()
	if err != nil {
		return err
	}

	fmt.Printf("'%s' is created and ready for deploy\n", targetFile)
	return nil
}

func (b *Build) createLaunchAgent(rootDir string) {
	launchDir := fmt.Sprintf("%s/Library/LaunchAgents/", rootDir)
	os.MkdirAll(launchDir, 0700)

	launchFile := fmt.Sprintf("%s/%s.kite.%s.plist", launchDir, b.Identifier, b.AppName)

	lFile, err := os.Create(launchFile)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("launchAgent").Parse(launchAgent))
	t.Execute(lFile, b)

}

func (b *Build) createDistribution(file string) {
	distFile, err := os.Create(file)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("distribution").Parse(distribution))
	t.Execute(distFile, b)

}

func (b *Build) createScripts(scriptDir string) {
	os.MkdirAll(scriptDir, 0700) // does return nil if exists

	postInstallFile, err := os.Create(scriptDir + "/postInstall")
	if err != nil {
		log.Fatalln(err)
	}
	postInstallFile.Chmod(0755)

	preInstallFile, err := os.Create(scriptDir + "/preInstall")
	if err != nil {
		log.Fatalln(err)
	}
	preInstallFile.Chmod(0755)

	t := template.Must(template.New("postInstall").Parse(postInstall))
	t.Execute(postInstallFile, b)

	t = template.Must(template.New("preInstall").Parse(preInstall))
	t.Execute(preInstallFile, b)
}

func fileExist(dir string) bool {
	var err error
	_, err = os.Stat(dir)
	if err == nil {
		return true // file exist
	}

	if os.IsNotExist(err) {
		return false // file does not exist
	}

	panic(err) // permission errors or something else bad
}
