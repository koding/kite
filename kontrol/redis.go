package kontrol

type Redis struct {
}

func (r *Redis) Get(key string) (Kites, error) {
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

func (r *Redis) Watch(key string, index uint64) (*Watcher, error) {
	return nil, nil
}
