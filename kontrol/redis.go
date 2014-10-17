package kontrol

import "github.com/koding/kite/protocol"

type Redis struct{}

func (r *Redis) Get(query *protocol.KontrolQuery) (Kites, error) {
	return nil, nil
}

func (r *Redis) Set(key, value string) error {
	return nil
}

func (r *Redis) Update(key, value string) error {
	return nil
}

func (r *Redis) Delete(key string) error {
	return nil
}
