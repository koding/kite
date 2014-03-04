// Package kitekey provides method for reading and writing kite.key file.
package kitekey

import (
	"io/ioutil"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"github.com/dgrijalva/jwt-go"
)

const (
	kiteDirName     = ".kite"
	kiteKeyFileName = "kite.key"
)

// KiteHome returns the home path of Kite directory.
// The returned value can be overriden by setting KITE_HOME environment variable.
func KiteHome() (string, error) {
	kiteHome := os.Getenv("KITE_HOME")
	if kiteHome != "" {
		return kiteHome, nil
	}
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
	keyPath, err := kiteKeyPath()
	if err != nil {
		return err
	}

	err = os.MkdirAll(filepath.Dir(keyPath), 0700)
	if err != nil {
		return err
	}

	// Need to remove the previous key first because we can't write over
	// when previos file's mode is 0400.
	os.Remove(keyPath)

	return ioutil.WriteFile(keyPath, []byte(kiteKey), 0400)
}

// Parse the kite.key file and return it as JWT token.
func Parse() (*jwt.Token, error) {
	kiteKey, err := Read()
	if err != nil {
		return nil, err
	}
	return jwt.Parse(kiteKey, GetKontrolKey)
}

// GetKontrolKey is used as key getter func for jwt.Parse() function.
func GetKontrolKey(token *jwt.Token) ([]byte, error) {
	return []byte(token.Claims["kontrolKey"].(string)), nil
}
