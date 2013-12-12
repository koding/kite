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
	scriptDir := "darwin/scripts"
	pkgContent := "pkgContent"

	tempDest, err := ioutil.TempDir("", "tempDest")
	if err != nil {
		return
	}
	defer os.RemoveAll(tempDest)

	cmdPkg := exec.Command("pkgbuild",
		"--identifier", "com.koding.kd.pkg",
		"--version", version,
		"--scripts", scriptDir,
		"--root", pkgContent,
		"--install-location", "/",
		tempDest+"/tmp.pkg", // this is used for second step
	)

	res, err := cmdPkg.CombinedOutput()
	if err != nil {
		fmt.Println("res, err", string(res), err)
		return
	}

	distribution := "darwin/Distribution"
	resources := "darwin/Resources"
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
