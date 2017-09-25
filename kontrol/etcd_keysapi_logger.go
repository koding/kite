package kontrol

import (
	"context"

	etcd "github.com/coreos/etcd/client"
	"github.com/koding/kite"
)

type KeysAPILogger struct {
	kapi etcd.KeysAPI
	log  kite.Logger
}

func NewKeysAPILogger(kapi etcd.KeysAPI, log kite.Logger) KeysAPILogger {
	return KeysAPILogger{
		kapi: kapi,
		log:  log,
	}
}

func (k KeysAPILogger) Get(ctx context.Context, key string, opts *etcd.GetOptions) (*etcd.Response, error) {
	k.log.Debug("Get: key: %v opts: %v", key, opts)
	return k.kapi.Get(ctx, key, opts)
}

func (k KeysAPILogger) Set(ctx context.Context, key, value string, opts *etcd.SetOptions) (*etcd.Response, error) {
	k.log.Debug("Set: key: %v value: %v opts: %v", key, value, opts)
	return k.kapi.Set(ctx, key, value, opts)
}

func (k KeysAPILogger) Delete(ctx context.Context, key string, opts *etcd.DeleteOptions) (*etcd.Response, error) {
	k.log.Debug("Delete: key: %v opts: %v", key, opts)
	return k.kapi.Delete(ctx, key, opts)
}

func (k KeysAPILogger) Create(ctx context.Context, key, value string) (*etcd.Response, error) {
	k.log.Debug("Create: key: %v value: %v", key, value)
	return k.kapi.Create(ctx, key, value)
}

func (k KeysAPILogger) CreateInOrder(ctx context.Context, dir, value string, opts *etcd.CreateInOrderOptions) (*etcd.Response, error) {
	k.log.Debug("CreateInOrder: dir: %v value: %v opts: %v", dir, value, opts)
	return k.kapi.CreateInOrder(ctx, dir, value, opts)
}

func (k KeysAPILogger) Update(ctx context.Context, key, value string) (*etcd.Response, error) {
	k.log.Debug("Update: key: %v value: %v", key, value)
	return k.kapi.Update(ctx, key, value)
}

func (k KeysAPILogger) Watcher(key string, opts *etcd.WatcherOptions) etcd.Watcher {
	k.log.Debug("Watcher: key: %v opts: %v", key, opts)
	return k.kapi.Watcher(key, opts)
}
