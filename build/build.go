package main

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
)

type Build struct{}

func main() {
	build := new(Build)
	build.darwin()
}

func (b *Build) darwin() {
	version := "1.0.0"
	scriptDir := "./darwin/scripts"
	installRoot := "./root"
	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDest)

	cmdPkg := exec.Command("pkgbuild",
		"--identifier", "com.koding.kd.pkg",
		"--version", version,
		"--scripts", scriptDir,
		"--root", installRoot,
		"--install-location", "/",
		tempDest+"/com.koding.kd.pkg", // used for next step, also set up for distribution.xml
	)

	res, err := cmdPkg.CombinedOutput()
	if err != nil {
		fmt.Println("res, err", string(res), err)
		return
	}

	distribution := "./darwin/Distribution.xml"
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
