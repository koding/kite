package deps

import (
	"encoding/json"
	"io"
	"os"
	"os/exec"
)

type Package struct {
	Dir        string
	ImportPath string
	Name       string
	Stale      string
	Root       string
	GoFiles    []string
	Imports    []string
	Deps       []string
}

func listPackages(pkgs ...Pkg) ([]*Package, error) {
	packages := make([]*Package, 0)
	args := []string{"list", "-e", "-json"}

	goPkgs := make([]string, len(pkgs))
	for i, p := range pkgs {
		goPkgs[i] = p.ImportPath
	}

	cmd := exec.Command("go", append(args, goPkgs...)...)
	cmd.Stderr = os.Stderr
	r, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}

	err = cmd.Start()
	if err != nil {
		return nil, err
	}

	d := json.NewDecoder(r)
	for {
		info := new(Package)
		err = d.Decode(info)
		if err == io.EOF {
			break
		}

		packages = append(packages, info)
	}

	err = cmd.Wait()
	if err != nil {
		return nil, err
	}

	return packages, nil
}
