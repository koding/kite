package main

import (
	"fmt"
	"kite/protocol"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

type MongoDB struct{}

func NewMongoDB() *MongoDB {
	return &MongoDB{}
}

func (m *MongoDB) Add(kite *protocol.Kite) {
	if kite.Id == "" {
		kite.Id = bson.NewObjectId()
	}

	err := mongo.UpsertKite(kite)
	if err != nil {
		fmt.Println("add kite err:", err)
	}
}

func (m *MongoDB) Get(id string) *protocol.Kite {
	kite, err := mongo.GetKite(id)
	if err != nil && err != mgo.ErrNotFound {
		fmt.Println("get kite err:", err)
	}
	return kite
}

func (m *MongoDB) Remove(id string) {
	err := mongo.DeleteKite(id)
	if err != nil {
		fmt.Println("delete kite err", err)
	}
}

func (m *MongoDB) Has(id string) bool {
	_, err := mongo.GetKite(id)
	if err == nil {
		return true
	}

	if err == mgo.ErrNotFound || err != nil {
		return false
	}

	return false
}

func (m *MongoDB) Size() int {
	n, err := mongo.SizeKites()
	if err != nil {
		fmt.Println("size kites err:", err)
	}
	return n
}

func (m *MongoDB) List() []*protocol.Kite {
	return mongo.ListKites()
}
