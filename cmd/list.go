package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/koding/kite/kitekey"
)

type List struct{}

func NewList() *List {
	return &List{}
}

func (*List) Definition() string {
	return "List installed kites"
}

func (*List) Exec(args []string) error {
	kites, err := getInstalledKites("")
	if err != nil {
		return err
	}

	for _, k := range kites {
		fmt.Println(k)
	}

	return nil
}

// getIntalledKites returns installed kites in .kd/kites folder.
// an empty argument returns all kites.
func getInstalledKites(kiteName string) ([]string, error) {
	kiteHome, err := kitekey.KiteHome()
	if err != nil {
		return nil, err
	}
	kitesPath := filepath.Join(kiteHome, "kites")

	domains, err := ioutil.ReadDir(kitesPath)
	if err != nil {
		if os.IsNotExist(err) {
			return []string{}, nil
		}

		return nil, err
	}

	installedKites := []string{} // to be returned

	for _, domain := range domains {
		usernames, err := ioutil.ReadDir(filepath.Join(kitesPath, domain.Name()))
		if err != nil {
			fmt.Println(err)
			continue
		}

		for _, username := range usernames {
			repos, err := ioutil.ReadDir(filepath.Join(kitesPath, domain.Name(), username.Name()))
			if err != nil {
				fmt.Println(err)
				continue
			}

			for _, repo := range repos {
				versions, err := ioutil.ReadDir(filepath.Join(kitesPath, domain.Name(), username.Name(), repo.Name()))
				if err != nil {
					fmt.Println(err)
					continue
				}

				for _, version := range versions {
					_, err := os.Stat(filepath.Join(kitesPath, domain.Name(), username.Name(), repo.Name(), version.Name(), "bin", strings.TrimSuffix(repo.Name(), ".kite")))
					if err != nil {
						fmt.Println(err)
						continue
					}

					fullName := filepath.Join(domain.Name(), username.Name(), repo.Name(), version.Name())
					installedKites = append(installedKites, fullName)
				}
			}
		}
	}

	return installedKites, nil
}
