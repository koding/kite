package kontrol

import (
	"fmt"
	"kite"
	"kite/protocol"
	"kite/testkeys"
	"kite/testutil"
	"testing"
	"time"
)

func TestKontrol(t *testing.T) {
	testutil.WriteKiteKey()

	opts := &kite.Options{
		Kitename:    "kontrol",
		Version:     "0.0.1",
		Region:      "localhost",
		Environment: "testing",
		PublicIP:    "127.0.0.1",
		Port:        "3999",
		Path:        "/kontrol",
	}
	kon := New(opts, "kontrol", "/tmp/test_kontrol_data_dir", nil, testkeys.Public, testkeys.Private)
	kon.Start()
	kon.ClearKites()

	mathKite := mathWorker()
	mathKite.Start()

	exp2Kite := exp2()
	exp2Kite.Start()

	// Wait for kites to register themselves on Kontrol.
	time.Sleep(500 * time.Millisecond)

	query := protocol.KontrolQuery{
		Username:    "testuser",
		Environment: "development",
		Name:        "mathworker",
	}

	kites, err := exp2Kite.Kontrol.GetKites(query)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	if len(kites) == 0 {
		t.Errorf("No mathworker available")
		return
	}

	mathWorker := kites[0]
	err = mathWorker.Dial()
	if err != nil {
		t.Errorf("Cannot connect to remote mathworker")
		return
	}

	// Test Kontrol.GetToken
	fmt.Printf("oldToken: %#v\n", mathWorker.Authentication.Key)
	newToken, err := exp2Kite.Kontrol.GetToken(&mathWorker.Kite)
	if err != nil {
		t.Errorf(err.Error())
	}
	fmt.Printf("newToken: %#v\n", newToken)

	response, err := mathWorker.Tell("square", 2)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	var result int
	err = response.Unmarshal(&result)
	if err != nil {
		t.Errorf(err.Error())
		return
	}

	if result != 4 {
		t.Errorf("Invalid result: %d", result)
		return
	}
}

func mathWorker() *kite.Kite {
	options := &kite.Options{
		Kitename:    "mathworker",
		Version:     "0.0.1",
		Port:        "3636",
		Region:      "localhost",
		Environment: "development",
	}

	k := kite.New(options)
	k.HandleFunc("square", Square)
	return k
}

func Square(r *kite.Request) (interface{}, error) {
	a, err := r.Args[0].Float64()
	if err != nil {
		return nil, err
	}

	result := a * a

	fmt.Printf("Kite call, sending result '%f' back\n", result)

	return result, nil
}

func exp2() *kite.Kite {
	options := &kite.Options{
		Kitename:    "exp2",
		Version:     "0.0.1",
		Port:        "3637",
		Region:      "localhost",
		Environment: "development",
	}

	return kite.New(options)
}

func TestGetQueryKey(t *testing.T) {
	// This query is valid because there are no gaps between query fields.
	q := &protocol.KontrolQuery{
		Username:    "cenk",
		Environment: "production",
	}
	key, err := getQueryKey(q)
	if err != nil {
		t.Errorf(err.Error())
	}
	if key != "/cenk/production" {
		t.Errorf("Unexpected key: %s", key)
	}

	// This is wrong because Environment field is empty.
	// We can't make a query on etcd because wildcards are not allowed in paths.
	q = &protocol.KontrolQuery{
		Username: "cenk",
		Name:     "fs",
	}
	key, err = getQueryKey(q)
	if err == nil {
		t.Errorf("Error is expected")
	}
	if key != "" {
		t.Errorf("Key is not expected: %s", key)
	}

	// This is also wrong becaus each query must have a non-empty username field.
	q = &protocol.KontrolQuery{
		Environment: "production",
		Name:        "fs",
	}
	key, err = getQueryKey(q)
	if err == nil {
		t.Errorf("Error is expected")
	}
	if key != "" {
		t.Errorf("Key is not expected: %s", key)
	}
}
