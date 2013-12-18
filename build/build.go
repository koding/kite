package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"text/template"
)

var binaryPath = flag.String("bin", "", "binary to be included into the package")

type Build struct {
	appName    string
	version    string
	output     string
	binaryPath string
}

func main() {
	flag.Parse()

	if *binaryPath == "" {
		fmt.Println("please specify application binary with --bin flag")
		os.Exit(1)
	}

	if !fileExist(*binaryPath) {
		fmt.Printf("specified binary doesn't exist: %s\n", *binaryPath)
		os.Exit(1)
	}

	// use binary name as appName
	appName := filepath.Base(*binaryPath)

	build := &Build{
		appName:    appName,
		version:    "0.0.1",
		binaryPath: *binaryPath,
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

// darwin is building a new .pkg installer for darwin based OS'es.
func (b *Build) darwin() {
	version := b.version
	if b.output == "" {
		b.output = fmt.Sprintf("koding-%s", b.appName)
	}

	scriptDir := "./darwin/scripts"
	installRoot := "./root" // TODO REMOVE

	os.RemoveAll(installRoot) // clean up old build before we continue

	installRootUsr := filepath.Join(installRoot, "/usr/local/bin")

	os.MkdirAll(installRootUsr, 0755)
	err = copyFile(b.binaryPath, installRootUsr+"/"+b.appName)
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

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

func copyFile(src, dst string) error {
	sf, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sf.Close()

	fi, err := sf.Stat()
	if err != nil {
		return err
	}

	if fi.IsDir() {
		return errors.New("src is a directory, please provide a file")
	}

	df, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, fi.Mode())
	if err != nil {
		return err
	}
	defer df.Close()

	if _, err := io.Copy(df, sf); err != nil {
		return err
	}

	return nil
}
