// Package sshutil implements ClientPassword and ClientKeyring
// interfaces from code.google.com/p/go.crypto/ssh package.
package sshutil

import (
	"io"
	"io/ioutil"

	"code.google.com/p/go.crypto/ssh"
)

// Password implements ssh.ClientPassword interface.
type Password string

func (p Password) Password(user string) (string, error) {
	return string(p), nil
}

// Keychain implements ssh.ClientKeyring interface.
type Keychain struct {
	keys []ssh.Signer
}

func (k *Keychain) Key(i int) (ssh.PublicKey, error) {
	if i < 0 || i >= len(k.keys) {
		return nil, nil
	}

	return k.keys[i].PublicKey(), nil
}

func (k *Keychain) Sign(i int, rand io.Reader, data []byte) (sig []byte, err error) {
	return k.keys[i].Sign(rand, data)
}

func (k *Keychain) Add(key ssh.Signer) {
	k.keys = append(k.keys, key)
}

func (k *Keychain) LoadPEMFile(file string) error {
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	return k.LoadPEM(buf)
}

func (k *Keychain) LoadPEM(data []byte) error {
	key, err := ssh.ParsePrivateKey(data)
	if err != nil {
		return err
	}
	k.Add(key)
	return nil
}
