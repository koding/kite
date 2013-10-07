package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/ioutil"
	"koding/db/mongodb"
	"koding/newkite/protocol"
	"koding/tools/slog"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"net/http"
)

// preparHandler first checks if the incoming POST request is a valid session.
// Every request made to kontrol should be in POST with protocol.Request in
// their body.
func prepareHandler(fn func(w http.ResponseWriter, r *http.Request, msg *protocol.Request)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")

		msg := new(protocol.Request)
		body, _ := ioutil.ReadAll(r.Body)
		err := json.Unmarshal(body, &msg)
		if err != nil {
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}

		slog.Printf("rx: sessionID '%s' wants '%s'\n", msg.SessionID, msg.RemoteKite)

		s, err := getSession(msg.SessionID)
		if err != nil {
			slog.Printf("i : sessionID '%s' is not validated (err: 1)\n", msg.SessionID)
			http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", err), http.StatusBadRequest)
			return
		}

		if s.ClientId != msg.SessionID {
			slog.Printf("i : sessionID '%s' is not validated (err: 2)\n", msg.SessionID)
			http.Error(w, "{\"err\":\"not authorized 1\"}\n", http.StatusBadRequest)
			return
		}

		slog.Printf("i : sessionID '%s' is validated as: %s\n", msg.SessionID, s.Username)

		msg.Username = s.Username
		fn(w, r, msg)
	}
}

func homeHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world - kontrol!\n")
}

func requestHandler(w http.ResponseWriter, r *http.Request, msg *protocol.Request) {
	list := make([]protocol.PubResponse, 0)
	var token *protocol.Token

	matchKite := msg.Username + "/" + msg.RemoteKite

	slog.Printf("i : searching for kite '%s'\n", matchKite)
	for _, k := range storage.List() {
		if k.Kitename == matchKite {
			token = getToken(msg.Username)
			if token == nil {
				token = createToken(msg.Username)
			}

			k.Token = token.ID // only token id is important for requester
			pubResp := createResponse(protocol.AddKite, k)
			list = append(list, pubResp)
		}
	}

	if len(list) == 0 {
		res := fmt.Sprintf("'%s' not available", matchKite)
		slog.Printf("i : %s", res)
		http.Error(w, fmt.Sprintf("{\"err\":\"%s\"}\n", res), http.StatusBadRequest)
		return
	}

	l, err := json.Marshal(list)
	if err != nil {
		slog.Println("i: marshalling kite list:", err)
		http.Error(w, "{\"err\":\"malformed kite list\"}\n", http.StatusBadRequest)
		return
	}

	slog.Printf("tx: sending token '%s' to be used with '%s' to '%s'\n", token.ID, msg.RemoteKite, msg.Username)
	w.Write([]byte(l))
}

// for now these fields are enough
type Session struct {
	Id       bson.ObjectId `bson:"_id" json:"-"`
	ClientId string        `bson:"clientId"`
	Username string        `bson:"username"`
	GuestId  int           `bson:"guestId"`
}

// TODO: move to models and modelhelpers
func getSession(token string) (*Session, error) {
	var session *Session

	query := func(c *mgo.Collection) error {
		return c.Find(bson.M{"clientId": token}).One(&session)
	}

	err := mongodb.Run("jSessions", query)
	if err != nil {
		return nil, err
	}

	return session, nil
}
