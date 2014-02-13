package protocol

import (
	"encoding/json"
	"net/url"
	"testing"
)

func TestMarshal(t *testing.T) {
	u := KiteURL{url.URL{
		Scheme: "http",
		Host:   "koding.com",
		Path:   "/",
	}}
	data, err := json.Marshal(&u)
	if err != nil {
		t.Errorf(err.Error())
		return
	}
	if string(data) != `"http://koding.com/"` {
		t.Errorf("unexpected: %s", string(data))
	}
}

func TestUnarshal(t *testing.T) {
	var u KiteURL
	err := json.Unmarshal([]byte(`"http://koding.com/"`), &u)
	if err != nil {
		t.Errorf(err.Error())
		return
	}
	if u.Scheme != "http" {
		t.Errorf("unexpected scheme: %s", u.Scheme)
	}
	if u.Host != "koding.com" {
		t.Errorf("unexpected host: %s", u.Host)
	}
	if u.Path != "/" {
		t.Errorf("unexpected path: %s", u.Path)
	}
}
