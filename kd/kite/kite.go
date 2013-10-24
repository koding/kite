// Package kite implements the "kd kite" sub-commands.
package kite

import (
	"path/filepath"
)

type Kite struct {
	KiteName string
	Folder   string
	// gives path for kite executable
	KiteExecutable string
}

func NewKite(kiteName string) *Kite {
	folder := kiteName + ".kite"
	kiteExecutable := "./" + filepath.Join(filepath.Join(folder, kiteName+"-kite"))
	return &Kite{
		KiteName:       kiteName,
		Folder:         folder,
		KiteExecutable: kiteExecutable,
	}
}
