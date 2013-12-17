package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"runtime"
	"text/template"
)

type Build struct{}

func main() {
	build := new(Build)
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
	const (
		postInstall = `#!/bin/bash

KITE_PLIST="/Library/LaunchAgents/com.koding.kite.{{.}}.plist"
chown root:wheel ${KITE_PLIST}
chmod 644 ${KITE_PLIST}

echo $USER
su $USER -c "/bin/launchctl load ${KITE_PLIST}"

exit 0
`

		preInstall = `#!/bin/sh

KDFILE=/usr/local/bin/{{.}}

echo "Removing previous installation"
if [ -f $KDFILE  ]; then
    rm -r $KDFILE
fi

echo "Checking for plist"
if /bin/launchctl list "com.koding.kite.{{.}}.plist" &> /dev/null; then
    echo "Unloading plist"
    /bin/launchctl unload "/Library/LaunchAgents/com.koding.kite.{{.}}.plist"
fi

exit 0
`
	)

	version := "1.0.0"
	scriptDir := "./darwin/scripts"
	installRoot := "./root"
	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDest)

	templatePost := template.Must(template.New("postInstall").Parse(postInstall))
	templatePost.Execute(os.Stdout, "fatih")

	templatePre := template.Must(template.New("preInstall").Parse(preInstall))
	templatePre.Execute(os.Stdout, "fatih")

	cmdPkg := exec.Command("pkgbuild",
		"--identifier", "com.koding.kite.pkg",
		"--version", version,
		"--scripts", scriptDir,
		"--root", installRoot,
		"--install-location", "/",
		tempDest+"/com.koding.kite.pkg", // used for next step, also set up for distribution.xml
	)

	res, err := cmdPkg.CombinedOutput()
	if err != nil {
		fmt.Println("res, err", string(res), err)
		return
	}

	distribution := "./darwin/Distribution.xml" // TODO: create it via a template
	resources := "./darwin/Resources"
	targetFile := "koding-kd-tool.pkg"

	cmdBuild := exec.Command("productbuild",
		"--distribution", distribution,
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
