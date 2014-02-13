package protocol

import (
	"encoding/json"
	"net/url"
)

// KiteURL is extended from url.URL to marshal/unmarshal as string.
type KiteURL struct{ url.URL }

func (u *KiteURL) MarshalJSON() ([]byte, error) {
	if u == nil {
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

	parsed, err := url.Parse(s)
	if err != nil {
		return err
	}

	u.URL = *parsed
	return nil
}
