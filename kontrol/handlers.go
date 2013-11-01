package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"koding/tools/slog"
	"net/http"
)

// everyone needs a place for home
func homeHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world - kontrol!\n")
}

// preparHandler first checks if the incoming POST request is a valid session.
// Every request made to kontrol should be in POST with protocol.BrowserToKontrolRequest in
// their body.
func prepareHandler(fn func(w http.ResponseWriter, r *http.Request, msg *protocol.BrowserToKontrolRequest)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		msg, err := readBrowserRequest(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}

		err = validatePostRequest(msg)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}
		slog.Printf("sessionID '%s' wants '%s'\n", msg.SessionID, msg.Kitename)

		session, err := modelhelper.GetSession(msg.SessionID)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}
		slog.Printf("sessionID '%s' is validated as: %s\n", msg.SessionID, session.Username)

		// username is used for matching kites and generating tokens in requestHandler
		msg.Username = session.Username

		fn(w, r, msg)
	}
}

// we assume that the incoming JSON data is in form of protocol.BrowserToKontrolRequest. Read
// and return a new protocol.BrowserToKontrolRequest from the POST body if succesfull.
func readBrowserRequest(requestBody io.ReadCloser) (*protocol.BrowserToKontrolRequest, error) {
	body, err := ioutil.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	defer requestBody.Close()

	var req protocol.BrowserToKontrolRequest
	err = json.Unmarshal(body, &req)
	if err != nil {
		return nil, err
	}

	return &req, nil
}

// validate that incoming post request has all necessary (at least the one we
// need) fields.
func validatePostRequest(msg *protocol.BrowserToKontrolRequest) error {
	if msg.SessionID == "" {
		return errors.New("sessionID field is empty")
	}

	if msg.Kitename == "" {
		return errors.New("kitename field is not specified")
	}

	return nil
}

// searchForKites returns a list of kites that matches the variable matchKite
// It also generates a new one-way token that is used between the client and
// kite and appends it to each kite struct
func searchForKites(username, kitename string) ([]protocol.KiteWithToken, error) {
	kites := make([]protocol.KiteWithToken, 0)

	slog.Printf("searching for kite '%s'\n", kitename)

	for _, k := range storage.List() {
		if k.Username == username && k.Name == kitename {
			key, err := kodingkey.FromString(k.KodingKey)
			if err != nil {
				return nil, fmt.Errorf("Koding Key is invalid at Kite: %s", k.Name)
			}

			// username is from requester, key is from kite owner
			tokenString, err := token.NewToken(username).EncryptString(key)
			if err != nil {
				return nil, errors.New("Server error: Cannot generate a token")
			}

			kwt := protocol.KiteWithToken{
				Kite:  k.Kite,
				Token: tokenString,
			}
			kites = append(kites, kwt)
		}
	}

	if len(kites) == 0 {
		return nil, fmt.Errorf("'%s' not available", kitename)
	}

	return kites, nil
}

// requestHandler sends as response a list of kites that matches kites in form
// of "username/kitename".
// Request comes from a web browser.
func requestHandler(w http.ResponseWriter, r *http.Request, msg *protocol.BrowserToKontrolRequest) {
	kites, err := searchForKites(msg.Username, msg.Kitename)
	if err != nil {
		msg := fmt.Sprintf("{\"err\":\"%s\"}\n", err)
		http.Error(w, msg, http.StatusBadRequest)
		return
	}

	kitesJSON, err := json.Marshal(kites)
	if err != nil {
		http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
		return
	}

	w.Write([]byte(kitesJSON))
}
