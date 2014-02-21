package build

import (
	"fmt"
	"io/ioutil"
	"github.com/koding/kite/cmd/util"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"text/template"
)

type Darwin struct {
	AppName    string
	BinaryPath string
	Version    string
	Output     string
	Identifier string
}

// Darwin is building a new .pkg installer for darwin based OS'es. It returns
// the created filename of the .pkg file.
func (d *Darwin) Build() (string, error) {
	installRoot, err := ioutil.TempDir(".", "kd-build-darwin_")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(installRoot)

	buildFolder, err := ioutil.TempDir(".", "kd-build-darwin_")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(buildFolder)

	scriptDir := filepath.Join(buildFolder, "scripts")
	installRootUsr := filepath.Join(installRoot, "/usr/local/bin")

	os.MkdirAll(installRootUsr, 0755)
	err = util.Copy(d.BinaryPath, installRootUsr+"/"+d.AppName)
	if err != nil {
		return "", err
	}

	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(tempDest)

	d.createScripts(scriptDir)
	d.createLaunchAgent(installRoot)

	cmdPkg := exec.Command("pkgbuild",
		"--identifier", fmt.Sprintf("%s.kite.%s.pkg", d.Identifier, d.AppName),
		"--version", d.Version,
		"--scripts", scriptDir,
		"--root", installRoot,
		"--install-location", "/",
		fmt.Sprintf("%s/%s.kite.%s.pkg", tempDest, d.Identifier, d.AppName),
		// used for next step, also set up for distribution.xml
	)

	_, err = cmdPkg.CombinedOutput()
	if err != nil {
		return "", err
	}

	distributionFile := filepath.Join(buildFolder, "Distribution.xml")
	resources := "build/darwin/Resources"
	targetFile := d.Output + ".pkg"

	d.createDistribution(distributionFile)

	cmdBuild := exec.Command("productbuild",
		"--distribution", distributionFile,
		"--resources", resources,
		"--package-path", tempDest,
		targetFile,
	)

	_, err = cmdBuild.CombinedOutput()
	if err != nil {
		return "", err
	}

	return targetFile, nil
}

func (d *Darwin) createLaunchAgent(rootDir string) {
	launchDir := fmt.Sprintf("%s/Library/LaunchAgents/", rootDir)
	os.MkdirAll(launchDir, 0700)

	launchFile := fmt.Sprintf("%s/%s.kite.%s.plist", launchDir, d.Identifier, d.AppName)

	lFile, err := os.Create(launchFile)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("launchAgent").Parse(launchAgent))
	t.Execute(lFile, d)

}

func (d *Darwin) createDistribution(file string) {
	distFile, err := os.Create(file)
	if err != nil {
		log.Fatalln(err)
	}

	t := template.Must(template.New("distribution").Parse(distribution))
	t.Execute(distFile, d)

}

func (d *Darwin) createScripts(scriptDir string) {
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
	t.Execute(postInstallFile, d)

	t = template.Must(template.New("preInstall").Parse(preInstall))
	t.Execute(preInstallFile, d)
}
