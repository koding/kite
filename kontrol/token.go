package main

import (
	uuid "github.com/nu7hatch/gouuid"
	"koding/newkite/protocol"
	"time"
)

var tokens = make(map[string]*protocol.Token)

func newToken(username string) *protocol.Token {
	return &protocol.Token{
		ID:        generateToken(),
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

func generateToken() string {
	id, _ := uuid.NewV4()
	return id.String()
}
