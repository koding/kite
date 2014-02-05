package cmd

import (
	"fmt"
	"kite/cmd/util"
)

type ShowKey struct{}

func NewShowKey() *ShowKey {
	return &ShowKey{}
}

func (v *ShowKey) Definition() string {
	return "Show registration key info"
}

func (v *ShowKey) Exec(args []string) error {
	token, err := util.ParseKiteKey()
	if err != nil {
		return err
	}

	for k, v := range token.Claims {
		fmt.Printf("%-15s%+v\n", k, v)
	}

	return nil
}
