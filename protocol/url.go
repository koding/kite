package protocol

import (
	"encoding/json"
	"net/url"
)

// KiteURL is extended from url.URL to marshal/unmarshal as string.
type KiteURL struct{ *url.URL }

func (u KiteURL) MarshalJSON() ([]byte, error) {
	if u.URL == nil {
		return json.Marshal(nil)
	}
	return json.Marshal(u.String())
}

func (u *KiteURL) UnmarshalJSON(data []byte) error {
	var s string
	err := json.Unmarshal(data, &s)
	if err != nil {
		return err
	}

	u.URL, err = url.Parse(s)
	return err
}
