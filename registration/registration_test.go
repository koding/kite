package registration

// import (
// 	"testing"

// 	"github.com/koding/kite/kontrol"
// )

// func init() {
// 	kon := kontrol.New(kiteOptions, name, dataDir, peers, publicKey, privateKey)
// 	kon.Start()
// }

// func TestRegistration(t *testing.T) {
// 	c := NewConfig()

// 	kon := NewKontrol()
// 	err := kon.Dial()
// 	if err != nil {
// 		t.Error(err)
// 		return
// 	}

// 	k := New("test", "1.0.0", c)
// 	k.HandleFunc("hello", hello)

// 	s := NewKiteServer(k)
// 	s.Start()
// 	defer s.Close()

// 	r := NewRegistration(k, kon)
// 	r.RegisterToKontrol()
// 	r.RegisterToProxy()
// 	r.RegisterToProxyAndKontrol()

// 	<-s.CloseNotify()
// }

// func hello(r *Request) (interface{}, error) {
// 	return "hello", nil
// }
