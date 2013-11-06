package main

import (
	"errors"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/protocol"
)

type query protocol.KontrolQuery

// Validate first tries to validate the incoming query. It returns the
// username and error of the
func (q *query) Validate() error {
	switch q.Authentication.Type {
	case "browser":
		return q.validateBrowser()
	case "kite":
		return q.validateKite()
	default:
		return errors.New("authentication type is not specified.")
	}
}

// validateBrowser is used to validate the incoming sessionId from a
// koding.com client/user. it returns the username if it belongs to a koding
// user.
func (q *query) validateBrowser() error {
	if q.Authentication.Key == "" {
		return errors.New("sessionID field is empty")
	}

	session, err := modelhelper.GetSession(q.Authentication.Key)
	if err != nil {
		return err
	}

	// now we know which the username belong to
	q.Username = session.Username
	return nil
}

// validateBrowser is used to validate the incoming kodingKey from a kite. it
// returns the username if it belongs to a koding user.
func (q *query) validateKite() error {
	return errors.New("validateKite is not implemented.")
}
