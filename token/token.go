// Package token implements the Token type used between Kites.
// Kontrol service is the generator and distributor of these tokens.
package token

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"koding/newkite/kodingkey"
	"time"
)

const DefaultTokenDuration = 1 * time.Hour

// Token is a type used between Kites and Kite clients.
// When a process wants to talk with a kite it asks to Kontrol.
// If the client is allowed, Kontrol gives a short lived token to it.
type Token struct {
	// ValidUntil is the time this token expires
	ValidUntil time.Time `json:"validUntil"`

	// Username is the person that this token belongs to.
	// It is the person who makes the request.
	Username string `json:"username"`

	// KiteID is the ID of the Kite that the request is sent to.
	KiteID string `json:"kiteID"`

	// TODO Ideas for later
	// ValidFor int // allowed number of requests
	// Access (access control list)
}

func NewToken(username, kiteID string) *Token {
	return NewTokenWithDuration(username, kiteID, DefaultTokenDuration)
}

func NewTokenWithDuration(username, kiteID string, d time.Duration) *Token {
	return &Token{
		Username:   username,
		KiteID:     kiteID,
		ValidUntil: time.Now().UTC().Add(d),
	}
}

func (t Token) IsValid(kiteID string) bool {
	return t.ValidUntil.After(time.Now().UTC()) && t.KiteID == kiteID
}

// EncryptString encrypts and URLencodes the token.
func (t Token) EncryptString(key kodingkey.KodingKey) (string, error) {
	ciphertext, err := t.Encrypt(key)
	if err != nil {
		return "", err
	}

	return base64.URLEncoding.EncodeToString(ciphertext), nil
}

// DecryptString decrypts a URLencoded string and returns a pointer to token.
func DecryptString(s string, key kodingkey.KodingKey) (*Token, error) {
	ciphertext, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}

	return Decrypt(ciphertext, key)
}

// Encrypt converts the token to JSON, encrypts it with the key and prepends
// the IV. Every encrypted token will be different because IV is randomly
// generated at the encryption time.
func (t Token) Encrypt(key kodingkey.KodingKey) ([]byte, error) {
	data, err := json.Marshal(t)
	if err != nil {
		panic(err)
	}

	ciphertext, err := EncryptAESCFBwithIV(data, key.Bytes32())
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

// Decrypt takes a slice of byte and decrypts it as a Token.
func Decrypt(data, key kodingkey.KodingKey) (*Token, error) {
	// Decrypt bytes
	plaintext, err := DecryptAESCFBwithIV(data, key.Bytes32())
	if err != nil {
		return nil, err
	}

	// Unmarshal JSON
	t := &Token{}
	err = json.Unmarshal(plaintext, t)
	if err != nil {
		return nil, fmt.Errorf("JSON decode error: %s", err)
	}

	return t, nil
}

// EncryptAESCFBwithIV is a wrapper around EncryptAESCFB that prepends a
// randomly generated IV in front of ciphertext and returns the ciphertext.
//
// The IV needs to be unique, but not secure. Therefore it's common to
// include it at the beginning of the ciphertext.
func EncryptAESCFBwithIV(plaintext, key []byte) ([]byte, error) {
	ciphertext := make([]byte, aes.BlockSize+len(plaintext))
	iv := ciphertext[:aes.BlockSize]
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return nil, err
	}

	err := EncryptAESCFB(ciphertext[aes.BlockSize:], plaintext, key, iv)
	if err != nil {
		return nil, err
	}

	return ciphertext, nil
}

func DecryptAESCFBwithIV(ciphertext, key []byte) ([]byte, error) {
	iv := ciphertext[:aes.BlockSize]
	encrypted := ciphertext[aes.BlockSize:]
	plaintext := make([]byte, len(encrypted))

	err := DecryptAESCFB(plaintext, encrypted, key, iv)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

func EncryptAESCFB(dst, src, key, iv []byte) error {
	aesBlockEncrypter, err := aes.NewCipher([]byte(key))
	if err != nil {
		return err
	}
	aesEncrypter := cipher.NewCFBEncrypter(aesBlockEncrypter, iv)
	aesEncrypter.XORKeyStream(dst, src)
	return nil
}

func DecryptAESCFB(dst, src, key, iv []byte) error {
	aesBlockDecrypter, err := aes.NewCipher([]byte(key))
	if err != nil {
		return nil
	}
	aesDecrypter := cipher.NewCFBDecrypter(aesBlockDecrypter, iv)
	aesDecrypter.XORKeyStream(dst, src)
	return nil
}
