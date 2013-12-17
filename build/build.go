package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"runtime"
	"text/template"
)

type Build struct {
	appName string
	version string
	output  string
}

func main() {
	build := &Build{
		appName: "kd",
		version: "0.0.1",
	}

	build.do()
}

func (b *Build) do() {
	switch runtime.GOOS {
	case "darwin":
		b.darwin()
	default:
		fmt.Printf("not supported os: %s.\n", runtime.GOOS)
	}
}

// darwin is building a new .pkg installer for darwin based OS'es. create a
// folder called "root", which will be used as the installer content.
func (b *Build) darwin() {
	version := b.version
	if b.output == "" {
		b.output = fmt.Sprintf("koding-%s", b.appName)
	}

	scriptDir := "./darwin/scripts"
	installRoot := "./root"

	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return
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

	res, err := cmdPkg.CombinedOutput()
	if err != nil {
		fmt.Println("res, err", string(res), err)
		return
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

	res, err = cmdBuild.CombinedOutput()
	if err != nil {
		fmt.Println("res, err", string(res), err)
		return
	}

	fmt.Println("everything is ok")

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

func dirExist(dir string) bool {
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
