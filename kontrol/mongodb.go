package main

import (
	"fmt"
	"koding/db/models"
	"koding/db/mongodb/modelhelper"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
)

type MongoDB struct{}

func NewMongoDB() *MongoDB {
	return &MongoDB{}
}

func (m *MongoDB) Add(kite *models.Kite) {
	if kite.Id == "" {
		kite.Id = bson.NewObjectId()
	}

	err := modelhelper.UpsertKite(kite)
	if err != nil {
		fmt.Println("add kite err:", err)
	}
}

func (m *MongoDB) Get(id string) *models.Kite {
	kite, err := modelhelper.GetKite(id)
	if err != nil && err != mgo.ErrNotFound {
		fmt.Println("get kite err:", err)
	}
	return kite
}

func (m *MongoDB) Remove(id string) {
	err := modelhelper.DeleteKite(id)
	if err != nil {
		fmt.Println("delete kite err", err)
	}
}

func (m *MongoDB) Has(id string) bool {
	_, err := modelhelper.GetKite(id)
	if err == nil {
		return true
	}

	if err == mgo.ErrNotFound || err != nil {
		return false
	}

	return false
}

func (m *MongoDB) Size() int {
	n, err := modelhelper.SizeKites()
	if err != nil {
		fmt.Println("size kites err:", err)
	}
	return n
}

func (m *MongoDB) List() []*models.Kite {
	return modelhelper.ListKites()
}
