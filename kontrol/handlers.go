package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"net/http"
)

// everyone needs a place for home
func homeHandler(w http.ResponseWriter, r *http.Request) {
	io.WriteString(w, "Hello world - kontrol!\n")
}

// errHandler is a convienent handler for not writing lots of duplicate http.Errro codes
func errHandler(fn func(http.ResponseWriter, *http.Request) ([]byte, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		kites, err := queryHandler(w, r)
		if err != nil {
			errMsg := fmt.Sprintf("{\"err\":\"%s\"}\n", err)
			http.Error(w, errMsg, http.StatusBadRequest)
		}

		w.Write([]byte(kites))
	}
}

// queryHandler sends as response a list of kites that matches kites specified
// with the incoming KontrolQuery struct.
func queryHandler(w http.ResponseWriter, r *http.Request) ([]byte, error) {
	w.Header().Set("Access-Control-Allow-Origin", "*")

	query, err := readBrowserRequest(r.Body)
	if err != nil {
		return nil, err
	}

	kites, err := query.ValidateQueryAndGetKites()
	if err != nil {
		return nil, err
	}

	kitesJSON, err := json.Marshal(kites)
	if err != nil {
		return nil, err
	}

	return kitesJSON, nil
}

// we assume that the incoming JSON data is in form of protocol.KontrolQuery.
// Read and return a new protocol.KontrolQuery from the POST body if
// successful.
func readBrowserRequest(requestBody io.ReadCloser) (*KontrolQuery, error) {
	body, err := ioutil.ReadAll(requestBody)
	if err != nil {
		return nil, err
	}
	defer requestBody.Close()

	var req KontrolQuery
	err = json.Unmarshal(body, &req)
	if err != nil {
		return nil, err
	}

	return &req, nil
}

// searchForKites returns a list of kites that matches the variable matchKite
// It also generates a new one-way token that is used between the client and
// kite and appends it to each kite struct
func searchForKites(username, kitename string) ([]protocol.KiteWithToken, error) {
	kites := make([]protocol.KiteWithToken, 0)

	log.Info("searching for kite '%s'", kitename)

	for _, k := range storage.List() {
		if k.Username == username && k.Name == kitename {
			key, err := kodingkey.FromString(k.KodingKey)
			if err != nil {
				return nil, fmt.Errorf("Koding Key is invalid at Kite: %s", k.Name)
			}

			// username is from requester, key is from kite owner
			tokenString, err := token.NewToken(username, k.ID).EncryptString(key)
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
