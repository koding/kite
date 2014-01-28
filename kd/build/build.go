package build

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"koding/kite/kd/util"
	"koding/tools/deps"
	"log"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"runtime"
	"text/template"

	"github.com/fatih/file"
)

type Build struct {
	appName    string
	version    string
	output     string
	binaryPath string
	importPath string
}

func NewBuild() *Build {
	return &Build{}
}

func (b *Build) Definition() string {
	return "Build deployable install packages"
}

func (b *Build) Exec(args []string) error {
	usage := "Usage: kd build --import <importPath> || --bin <binaryPath>"

	if len(args) == 0 {
		return errors.New(usage)
	}

	if len(args) == 2 && (args[0] != "--bin" || args[0] != "--import") {
		return errors.New(usage)
	}

	build := &Build{
		version: "0.0.1",
	}

	switch args[0] {
	case "--bin":
		b.binaryPath = args[1]
	case "--import":
		b.importPath = args[1]
	}

	b.appName = filepath.Base(args[0])

	err := build.do()
	if err != nil {
		return err
	}

	fmt.Println("build successfull")
	return nil
}

func (b *Build) do() error {
	switch runtime.GOOS {
	case "darwin":
		return b.darwin()
	default:
		return fmt.Errorf("not supported os: %s.\n", runtime.GOOS)
	}
}

func (b *Build) linux() error {
	gopath := os.Getenv("GOPATH")
	if gopath == "" {
		return errors.New("GOPATH is not set")
	}

	pkgname := path.Base(b.importPath)

	// or use "go list koding/..." for all packages and commands
	packages := []string{b.importPath}

	d, err := deps.LoadDeps(packages...)
	if err != nil {
		return err
	}

	err = d.WriteJSON()
	if err != nil {
		return err
	}

	out, err := json.MarshalIndent(d, "", "  ")
	if err != nil {
		return err
	}

	fmt.Printf("installing packages to '%s'\n%s\n", d.BuildGoPath, string(out))
	err = d.InstallDeps()
	if err != nil {
		return err
	}

	packagePath := filepath.Join(d.BuildGoPath, pkgname)

	// prepare config folder
	// configPath := filepath.Join(packagePath, "config")
	// os.MkdirAll(configPath, 0755)

	// config, err := exec.Command("node", "-e", "require('koding-config-manager').printJson('main."+*profile+"')").CombinedOutput()
	// if err != nil {
	// 	return err
	// }

	// configFile := fmt.Sprintf("%s/main.%s.json", configPath, *profile)
	// err = ioutil.WriteFile(configFile, config, 0755)
	// if err != nil {
	// 	return err
	// }

	// copy package files, such as templates
	assets := []string{filepath.Join(gopath, "src", b.importPath, "files")}
	for _, assetDir := range assets {
		err := file.Copy(assetDir, packagePath)
		if err != nil {
			log.Println("copy assets", err)
		}
	}

	// create tar.gz file from final director
	tarFile := fmt.Sprintf("%s.%s-%s.tar.gz", pkgname, runtime.GOOS, runtime.GOARCH)
	err = util.MakeTar(tarFile, packagePath)
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

	scriptDir := "./darwin/scripts"
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

	distributionFile := "./darwin/Distribution.xml"
	resources := "./darwin/Resources"
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
