package kontrol

import (
	"testing"

	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
	uuid "github.com/satori/go.uuid"
)

func BenchmarkPostgres(b *testing.B) {
	kon.SetStorage(NewPostgres(nil, kon.Kite.Log))

	newKite := func() *protocol.Kite {
		id := uuid.NewV4()
		return &protocol.Kite{
			Username:    "bench-user",
			Environment: "bench-env",
			Name:        "mathworker",
			Version:     "1.1.1",
			Region:      "bench",
			Hostname:    "bench-host",
			ID:          id.String(),
		}
	}

	value := &kontrolprotocol.RegisterValue{
		URL: "http://localhost:4444/kite",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		kon.storage.Add(newKite(), value)
	}
}

func BenchmarkPostgresGet(b *testing.B) {
	kon.SetStorage(NewPostgres(nil, kon.Kite.Log))

	query := &protocol.KontrolQuery{
		ID: "b9cc3baf-4f03-47d0-5a62-7de2e9f22476",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		kon.storage.Get(query)
	}
}

func BenchmarkEtcdAdd(b *testing.B) {
	kon.SetStorage(NewEtcd(nil, kon.Kite.Log))

	newKite := func() *protocol.Kite {
		id := uuid.NewV4()
		return &protocol.Kite{
			Username:    "bench-user",
			Environment: "bench-env",
			Name:        "mathworker",
			Version:     "1.1.1",
			Region:      "bench",
			Hostname:    "bench-host",
			ID:          id.String(),
		}
	}

	value := &kontrolprotocol.RegisterValue{
		URL: "http://localhost:4444/kite",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		kon.storage.Add(newKite(), value)
	}
}

func BenchmarkEtcdGet(b *testing.B) {
	kon.SetStorage(NewEtcd(nil, kon.Kite.Log))

	query := &protocol.KontrolQuery{
		ID: "b9cc3baf-4f03-47d0-5a62-7de2e9f22476",
	}

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		kon.storage.Get(query)
	}
}
