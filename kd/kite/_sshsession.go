package kd

import (
	"bytes"
	"code.google.com/p/go.crypto/ssh"
	"crypto"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"io"
	"io/ioutil"
	"os/user"
)

type keychain struct {
	keys []interface{}
}

func (k *keychain) Key(i int) (interface{}, error) {
	if i < 0 || i >= len(k.keys) {
		return nil, nil
	}
	switch key := k.keys[i].(type) {
	case *rsa.PrivateKey:
		return &key.PublicKey, nil
	}
	return nil, errors.New("ssh: unknown key type")
}

func (k *keychain) Sign(i int, rand io.Reader, data []byte) (sig []byte, err error) {
	hashFunc := crypto.SHA1
	h := hashFunc.New()
	h.Write(data)
	digest := h.Sum(nil)
	switch key := k.keys[i].(type) {
	case *rsa.PrivateKey:
		return rsa.SignPKCS1v15(rand, key, hashFunc, digest)
	}
	return nil, errors.New("ssh: unknown key type")
}

func (k *keychain) LoadPEM(file string) error {
	buf, err := ioutil.ReadFile(file)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(buf)
	if block == nil {
		return errors.New("ssh: no key found")
	}
	r, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	k.keys = append(k.keys, r)
	return nil
}

type clientPassword string

func (p clientPassword) Password(user string) (string, error) {
	return string(p), nil
}

type SSHClient struct {
	Client  *ssh.ClientConn
	Session *ssh.Session
	Host    string
}
type SSHSession struct {
	Session *ssh.Session
}

func NewSSHClient(host string) *SSHClient {
	return &SSHClient{Host: host}
}

func (c *SSHClient) SetCredentialAuth(username, password string) error {
	config := &ssh.ClientConfig{
		User: username,
		Auth: []ssh.ClientAuth{
			// ClientAuthPassword wraps a ClientPassword implementation
			// in a type that implements ClientAuth.
			ssh.ClientAuthPassword(clientPassword(password)),
		},
	}
	client, err := ssh.Dial("tcp", c.Host+":22", config)
	c.Client = client
	return err
}

func (c *SSHClient) SetKeychainAuth(filepath string) error {
	key := new(keychain)
	err := key.LoadPEM(filepath)
	if err != nil {
		return err
	}
	currUser, err := user.Current()
	if err != nil {
		return err
	}
	config := &ssh.ClientConfig{
		User: currUser.Username,
		Auth: []ssh.ClientAuth{
			ssh.ClientAuthKeyring(key),
		},
	}
	client, err := ssh.Dial("tcp", c.Host+":22", config)
	c.Client = client
	return err
}

func (c *SSHClient) newSession() (error, *SSHSession) {
	// Each ClientConn can support multiple interactive sessions,
	// represented by a Session.
	session, err := c.Client.NewSession()
	return err, &SSHSession{Session: session}
}

func (s *SSHSession) Execute(command string) (string, error) {
	// Once a Session is created, you can execute a single command on
	// the remote side using the Run method.
	var o bytes.Buffer
	s.Session.Stdout = &o
	err := s.Session.Run(command)
	return o.String(), err
}

func (s *SSHSession) Close() error {
	return s.Session.Close()
}
