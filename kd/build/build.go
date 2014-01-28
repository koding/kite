package build

import (
	"errors"
	"flag"
	"fmt"
	"io/ioutil"
	"koding/kite/kd/util"
	"koding/tools/deps"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"text/template"

	"github.com/fatih/file"
)

type Build struct {
	appName    string
	version    string
	output     string
	binaryPath string
	importPath string
	files      string
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
	f.StringVar(&b.importPath, "import", "", "Go importpath to be packaged")
	f.StringVar(&b.binaryPath, "bin", "", "Binary to be packaged")
	f.StringVar(&b.files, "files", "", "Files to be included with the package")
	f.Parse(args)

	if b.binaryPath != "" {
		b.appName = filepath.Base(b.binaryPath)
	} else if b.importPath != "" {
		b.appName = filepath.Base(b.importPath)
	} else {
		return errors.New("build: --import or --bin should be defined.")
	}

	b.version = "0.0.1"
	b.output = fmt.Sprintf("%s.%s-%s", b.appName, runtime.GOOS, runtime.GOARCH)

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

	if b.importPath != "" {
		gopath := os.Getenv("GOPATH")
		if gopath == "" {
			return errors.New("GOPATH is not set")
		}

		// or use "go list koding/..." for all packages and commands
		packages := []string{b.importPath}
		d, err := deps.LoadDeps(packages...)
		if err != nil {
			return err
		}

		err = d.InstallDeps()
		if err != nil {
			return err
		}

		buildFolder = filepath.Join(d.BuildGoPath, b.appName)
	} else {
		err := file.Copy(b.binaryPath, buildFolder)
		if err != nil {
			log.Println("copy assets", err)
		}
	}

	// include given files
	if b.files != "" {
		files := strings.Split(b.files, ",")
		for _, path := range files {
			err := file.Copy(path, buildFolder)
			if err != nil {
				log.Println("copy assets", err)
			}
		}
	}

	// create tar.gz file from final director
	tarFile := fmt.Sprintf("%s.%s-%s.tar.gz", b.appName, runtime.GOOS, runtime.GOARCH)
	err = util.MakeTar(tarFile, buildFolder)
	if err != nil {
		return err
	}

	fmt.Printf("'%s' is created and ready for deploy\n", tarFile)
	return nil
}

// darwin is building a new .pkg installer for darwin based OS'es.
func (b *Build) darwin() error {
	version := b.version
	if b.output == "" {
		b.output = fmt.Sprintf("koding-%s", b.appName)
	}

	scriptDir := "build/darwin/scripts"
	installRoot := "./root" // TODO REMOVE

	os.RemoveAll(installRoot) // clean up old build before we continue
	installRootUsr := filepath.Join(installRoot, "/usr/local/bin")

	os.MkdirAll(installRootUsr, 0755)
	err := util.CopyFile(b.binaryPath, installRootUsr+"/"+b.appName)
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
		"--identifier", fmt.Sprintf("com.koding.kite.%s.pkg", b.appName),
		"--version", version,
		"--scripts", scriptDir,
		"--root", installRoot,
		"--install-location", "/",
		fmt.Sprintf("%s/com.koding.kite.%s.pkg", tempDest, b.appName),
		// used for next step, also set up for distribution.xml
	)

	_, err = cmdPkg.CombinedOutput()
	if err != nil {
		return err
	}

	distributionFile := "build/darwin/Distribution.xml"
	resources := "build/darwin/Resources"
	targetFile := b.output + ".pkg"

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

	launchFile := fmt.Sprintf("%s/com.koding.kite.%s.plist", launchDir, b.appName)

	lFile, err := os.Create(launchFile)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("launchAgent").Parse(launchAgent))
	t.Execute(lFile, b.appName)

}

func (b *Build) createDistribution(file string) {
	distFile, err := os.Create(file)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("distribution").Parse(distribution))
	t.Execute(distFile, b.appName)

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
	t.Execute(postInstallFile, b.appName)

	t = template.Must(template.New("preInstall").Parse(preInstall))
	t.Execute(preInstallFile, b.appName)
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
