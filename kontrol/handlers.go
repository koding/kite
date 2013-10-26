package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"net/http"
	"time"
)

// everyone needs a place for home
func homeHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world - kontrol!\n")
}

// preparHandler first checks if the incoming POST request is a valid session.
// Every request made to kontrol should be in POST with protocol.Request in
// their body.
func prepareHandler(fn func(w http.ResponseWriter, r *http.Request, msg *protocol.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		msg, err := readPostRequest(r.Body)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}

		err = validatePostRequest(msg)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}
		slog.Printf("sessionID '%s' wants '%s'\n", msg.SessionID, msg.RemoteKite)

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

// we assume that the incoming JSON data is in form of protocol.Request. Read
// and return a new protocol.Request from the POST body if succesfull.
func readPostRequest(requestBody io.ReadCloser) (*protocol.Request, error) {
	body, err := ioutil.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	defer requestBody.Close()

	req, err := unmarshalRequest(body)
	if err != nil {
		return nil, err
	}

	return req, nil
}

// validate that incoming post request has all necessary (at least the one we
// need) fields.
func validatePostRequest(msg *protocol.Request) error {
	if msg.SessionID == "" {
		return errors.New("sessionID field is empty")
	}

	if msg.RemoteKite == "" {
		return errors.New("remoteKite field is not specified")
	}

	return nil
}

// searchForKites returns a list of kites that matches the variable matchKite
// It also generates a new one-way token that is used between the client and
// kite and appends it to each kite struct
func searchForKites(username, kitename string) ([]protocol.PubResponse, error) {
	var err error
	kites := make([]protocol.PubResponse, 0)
	token := new(models.KiteToken)

	slog.Printf("searching for kite '%s'\n", kitename)

	for _, k := range storage.List() {
		if k.Username == username && k.Kitename == kitename {
			token, err = modelhelper.GetKiteToken(username)
			if err != nil || token == nil {

				// Token expire duration needs to be talked, for now it's two hours
				// token = modelhelper.NewKiteToken(username, time.Now().Add(2*time.Hour))
				token = modelhelper.NewKiteToken(username, time.Now().Add(20*time.Second))
			}

			token.Kites = append(token.Kites, k.Uuid)
			modelhelper.AddKiteToken(token) // updates if document is available

			k.Token = token.Token // only token id is important for client
			pubResp := createResponse(protocol.AddKite, k)
			kites = append(kites, pubResp)
		}
	}

	if len(kites) == 0 {
		return nil, fmt.Errorf("'%s' not available\n", kitename)
	}

	return kites, nil
}

// requestHandler sends as response a list of kites that matches kites in form
// of "username/kitename".
func requestHandler(w http.ResponseWriter, r *http.Request, msg *protocol.Request) {
	kites, err := searchForKites(msg.Username, msg.RemoteKite)
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
