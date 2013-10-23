package cli

import "fmt"

// Please use 3 digit versioning (major, minor, patch).
// http://semver.org
const VERSION = "0.0.1"

type Version struct{}

func NewVersion() *Version {
	return &Version{}
}

func (v Version) Definition() string {
	return "Show version"
}

func (v Version) Exec() error {
	fmt.Println(VERSION)
	return nil
}
