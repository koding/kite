package cmd

import (
	"fmt"

	"github.com/koding/kite/kitekey"
)

type ShowKey struct{}

func NewShowKey() *ShowKey {
	return &ShowKey{}
}

func (v *ShowKey) Definition() string {
	return "Show registration key info"
}

func (v *ShowKey) Exec(args []string) error {
	token, err := kitekey.Parse()
	if err != nil {
		return err
	}

	for k, v := range token.Claims {
		fmt.Printf("%-15s%+v\n", k, v)
	}

	return nil
}
