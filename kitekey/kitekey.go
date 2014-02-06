package kitekey

import (
	"github.com/dgrijalva/jwt-go"
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"
)

const (
	kiteDirName        = ".kite"
	kiteKeyFileName    = "kite.key"
	kontrolKeyFileName = "kontrol.key"
)

// KiteHome returns the home path of Kite directory.
func KiteHome() (string, error) {
	usr, err := user.Current()
	if err != nil {
		return "", err
	}
	return filepath.Join(usr.HomeDir, kiteDirName), nil
}

func kiteKeyPath() (string, error) {
	kiteHome, err := KiteHome()
	if err != nil {
		return "", err
	}
	return filepath.Join(kiteHome, kiteKeyFileName), nil
}

// Read the contents of the kite.key file.
func Read() (string, error) {
	keyPath, err := kiteKeyPath()
	if err != nil {
		return "", err
	}
	data, err := ioutil.ReadFile(keyPath)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// Write over the kite.key file.
func Write(kiteKey string) error {
	kiteHome, err := KiteHome()
	if err != nil {
		return err
	}
	os.Mkdir(kiteHome, 0700) // create if not exists
	path := filepath.Join(kiteHome, kiteKeyFileName)
	os.Remove(path)
	return ioutil.WriteFile(path, []byte(kiteKey), 0400)
}

// Parse the kite.key file and return it as JWT token.
func Parse() (*jwt.Token, error) {
	kiteKey, err := Read()
	if err != nil {
		return nil, err
	}
	return jwt.Parse(kiteKey, getKontrolKey)
}

// getKontrolKey is used as key getter func for jwt.Parse() function.
func getKontrolKey(token *jwt.Token) ([]byte, error) {
	return []byte(token.Claims["kontrolKey"].(string)), nil
}
