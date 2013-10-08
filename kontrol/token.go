package main

import (
	"koding/newkite/protocol"
	"koding/newkite/utils"
	"time"
)

var tokens = make(map[string]*protocol.Token)

func newToken(username string) *protocol.Token {
	return &protocol.Token{
		ID:        utils.GenerateUUID(),
		Username:  username,
		Expire:    0,
		CreatedAt: time.Now(),
	}
}

func getToken(username string) *protocol.Token {
	token, ok := tokens[username]
	if !ok {
		return nil
	}

	return token
}

func createToken(username string) *protocol.Token {
	t := newToken(username)
	tokens[username] = t
	return t
}

func deleteToken(username string) {
	delete(tokens, username)
}
