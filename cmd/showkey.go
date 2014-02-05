package cmd

import (
	"fmt"
	"github.com/dgrijalva/jwt-go"
	"koding/kite/cmd/util"
)

type ShowKey struct{}

func NewShowKey() *ShowKey {
	return &ShowKey{}
}

func (v *ShowKey) Definition() string {
	return "Show registration key info"
}

func (v *ShowKey) Exec(args []string) error {
	kiteKey, err := util.KiteKey()
	if err != nil {
		return err
	}

	token, err := jwt.Parse(kiteKey, getKontrolKey)
	if err != nil {
		return err
	}

	for k, v := range token.Claims {
		fmt.Printf("%-15s%+v\n", k, v)
	}

	return nil
}

func getKontrolKey(token *jwt.Token) ([]byte, error) {
	kontrolKey, err := util.KontrolKey()
	if err != nil {
		return nil, err
	}
	return []byte(kontrolKey), nil
}
