package kitetest

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"os/user"
	"time"

	"github.com/dgrijalva/jwt-go"
	"github.com/koding/kite/protocol"
	"github.com/satori/go.uuid"
)

// KeyPair represents PEM encoded RSA key pair.
type KeyPair struct {
	Public  []byte
	Private []byte
}

// KiteKey represents JWT token used in kite authentication,
// also known as kite.key.
type KiteKey struct {
	ID         string        `json:"keyID,omitempty"`
	Issuer     string        `json:"issuer,omitempty"`
	Username   string        `json:"username,omitempty"`
	IssuedAt   int64         `json:"issuedAt,omitempty"`
	KontrolURL string        `json:"kontrolURL,omitempty"`
	URL        string        `json:"url,omitempty"`
	Kite       protocol.Kite `json:"kite,omitempty"`
}

func (k *KiteKey) id() string {
	if k.ID != "" {
		return k.ID
	}
	return uuid.NewV4().String()
}

func (k *KiteKey) issuer() string {
	if k.Issuer != "" {
		return k.Issuer
	}
	return "koding"
}

func (k *KiteKey) username() string {
	if k.Username != "" {
		return k.Username
	}

	u, err := user.Current()
	if err != nil {
		panic(err)
	}

	return u.Username
}

func (k *KiteKey) issuedAt() int64 {
	if k.IssuedAt != 0 {
		return k.IssuedAt
	}
	return time.Now().UTC().Unix()
}

func (k *KiteKey) kontrolURL() string {
	if k.KontrolURL != "" {
		return k.KontrolURL
	}
	return "https://koding.com/kontrol/kite"
}

// GenerateKeyPair
func GenerateKeyPair() (*KeyPair, error) {
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, err
	}

	pub, err := x509.MarshalPKIXPublicKey(&priv.PublicKey)
	if err != nil {
		return nil, err
	}

	return &KeyPair{
		Private: pem.EncodeToMemory(&pem.Block{
			Type:  "RSA PRIVATE KEY",
			Bytes: x509.MarshalPKCS1PrivateKey(priv),
		}),
		Public: pem.EncodeToMemory(&pem.Block{
			Type:  "PUBLIC KEY",
			Bytes: pub,
		}),
	}, nil
}

// GenerateKiteKey
func GenerateKiteKey(k *KiteKey, keys *KeyPair) (*jwt.Token, error) {
	var err error

	if keys == nil {
		keys, err = GenerateKeyPair()
		if err != nil {
			return nil, err
		}
	}

	kiteKey := jwt.New(jwt.GetSigningMethod("RS256"))

	kiteKey.Claims = jwt.MapClaims{
		"iss":        k.issuer(),
		"sub":        k.username(),
		"iat":        k.issuedAt(),
		"jti":        k.id(),
		"kontrolURL": k.kontrolURL(),
		"kontrolKey": string(keys.Public),
	}

	privateKey, err := jwt.ParseRSAPrivateKeyFromPEM(keys.Private)
	if err != nil {
		return nil, err
	}

	kiteKey.Raw, err = kiteKey.SignedString(privateKey)
	if err != nil {
		return nil, err
	}

	kiteKey.Valid = true

	return kiteKey, nil
}

// TokenExtractor is used to extract kite ID from the given JWT token.
type TokenExtractor struct {
	Token  *jwt.Token
	KiteID string
}

var errTokenExtractor = errors.New("extraction ok")

// Extract is a jwt.Keyfunc that extracts kite ID.
func (te *TokenExtractor) Extract(tok *jwt.Token) (interface{}, error) {
	var id string
	var ok bool

	switch claims := tok.Claims.(type) {
	case *jwt.StandardClaims:
		id, ok = claims.Id, true
	case jwt.MapClaims:
		id, ok = claims["jti"].(string)
	}

	if !ok || id == "" {
		return nil, errors.New("no kite ID")
	}

	te.KiteID = id
	te.Token = tok

	return nil, errTokenExtractor
}

// ExtractKiteID extracts kite ID from the raw JWT token.
func ExtractKiteID(token string) (string, error) {
	te := &TokenExtractor{}

	_, err := jwt.Parse(token, te.Extract)

	if e, ok := err.(*jwt.ValidationError); ok && e.Error() == errTokenExtractor.Error() {
		return te.KiteID, nil
	}

	return "", err
}
