package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"net/http"
)

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

		msg.Username = session.Username
		fn(w, r, msg)
	}
}

func readPostRequest(requestBody io.ReadCloser) (*protocol.Request, error) {
	msg := new(protocol.Request)
	body, err := ioutil.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	defer requestBody.Close()

	err = json.Unmarshal(body, &msg)
	if err != nil {
		return nil, err
	}

	return msg, nil
}

func validatePostRequest(msg *protocol.Request) error {
	if msg.SessionID == "" {
		return errors.New("sessionID field is empty")
	}

	if msg.RemoteKite == "" {
		return errors.New("remoteKite field is not specified")
	}

	return nil
}

func requestHandler(w http.ResponseWriter, r *http.Request, msg *protocol.Request) {
	kites := make([]protocol.PubResponse, 0)
	var token *protocol.Token

	matchKite := msg.Username + "/" + msg.RemoteKite

	slog.Printf("searching for kite '%s'\n", matchKite)
	for _, k := range storage.List() {
		if k.Kitename == matchKite {
			token = getToken(msg.Username)
			if token == nil {
				token = createToken(msg.Username)
			}

			k.Token = token.ID // only token id is important for requester
			pubResp := createResponse(protocol.AddKite, k)
			kites = append(kites, pubResp)
		}
	}

	if len(kites) == 0 {
		res := fmt.Sprintf("'%s' not available\n", matchKite)
		slog.Printf(res)
		http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", res), http.StatusBadRequest)
		return
	}

	kitesJSON, err := json.Marshal(kites)
	if err != nil {
		slog.Println("marshalling kite list:", err)
		http.Error(w, "{\"err\":\"malformed kite list\"}\n", http.StatusBadRequest)
		return
	}

	slog.Printf("sending token '%s' to be used with '%s' to '%s'\n",
		token.ID, msg.RemoteKite, msg.Username)

	w.Write([]byte(kitesJSON))
}
