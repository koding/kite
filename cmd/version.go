package cmd

import "fmt"

type Version string

func (v Version) Definition() string {
	return "Show version of this command"
}

func (v Version) Exec(args []string) error {
	fmt.Println(v)
	return nil
}
