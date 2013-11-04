package token

import (
	"crypto/aes"
	"fmt"
	"koding/newkite/kodingkey"
	"testing"
	"time"
)

func TestEncodeDecodeString(t *testing.T) {
	key, err := kodingkey.NewKodingKey()
	if err != nil {
		t.Error(err)
		return
	}

	tok := NewToken("cenk", "1")
	fmt.Println("Generated new token:", *tok)

	enc, err := tok.EncryptString(key)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("Token encrypted:", enc)

	dec, err := DecryptString(enc, key)
	if err != nil {
		t.Error("Cannot decrypt token:", err)
		return
	}
	fmt.Println("Token decrypted:", dec)

	if dec.Username != "cenk" {
		t.Error("oops")
		return
	}
	fmt.Println("Username is correct")

	if dec.ValidUntil != tok.ValidUntil {
		t.Error("oops")
		return
	}
	fmt.Println("Timestamps are correct")
}

func TestEncryptDecrypt(t *testing.T) {
	key, err := kodingkey.NewKodingKey()
	if err != nil {
		t.Error(err)
		return
	}

	tok := NewToken("cenk", "1")
	fmt.Println("Generated new token:", *tok)

	enc, err := tok.Encrypt(key)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Println("Token encrypted:", enc)

	dec, err := Decrypt(enc, key)
	if err != nil {
		t.Error("Cannot decrypt token:", err)
		return
	}
	fmt.Println("Token decrypted:", dec)

	if dec.ValidUntil != tok.ValidUntil {
		t.Error("oops")
		return
	}
	fmt.Println("Timestamps are correct")
}

func TestTimestamp(t *testing.T) {
	tok := NewToken("cenk", "1")

	if !tok.IsValid("1") {
		t.Error("Should be valid")
		return
	}

	tok = NewTokenWithDuration("cenk", "1", -10*time.Millisecond)

	if tok.IsValid("1") {
		t.Error("Should not be valid")
		return
	}
}

func TestAESCFB(t *testing.T) {
	const key16 = "1234567890123456"
	const key24 = "123456789012345678901234"
	const key32 = "12345678901234567890123456789012"
	var key = key16
	var msg = "message"
	var iv = []byte(key)[:aes.BlockSize] // Using IV same as key is probably bad
	var err error

	// Encrypt
	encrypted := make([]byte, len(msg))
	err = EncryptAESCFB(encrypted, []byte(msg), []byte(key), iv)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("Encrypting %v %s -> %v\n", []byte(msg), msg, encrypted)

	// Decrypt
	decrypted := make([]byte, len(msg))
	err = DecryptAESCFB(decrypted, encrypted, []byte(key), iv)
	if err != nil {
		t.Error(err)
		return
	}
	fmt.Printf("Decrypting %v -> %v %s\n", encrypted, decrypted, decrypted)
}
