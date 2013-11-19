package main

import (
	"errors"
	"fmt"
	"koding/db/models"
	"koding/db/mongodb"
	"koding/db/mongodb/modelhelper"
	"koding/newkite/kodingkey"
	"koding/newkite/protocol"
	"koding/newkite/token"
	"labix.org/v2/mgo"
	"reflect"
	"strings"
)

type KontrolQuery protocol.KontrolQuery

// Run runs the query and returns matching Kites with attached tokens for authentication.
func (k *KontrolQuery) Run() ([]protocol.KiteWithToken, error) {
	return k.kiteWithTokens(k.Username)
}

func (k *KontrolQuery) kiteWithTokens(requester string) ([]protocol.KiteWithToken, error) {
	queriedKites := k.query()
	if len(queriedKites) == 0 {
		return nil, fmt.Errorf("'%s' for username '%s' not available",
			k.Name, k.Username)
	}

	kitesWithToken := make([]protocol.KiteWithToken, 0)

	for _, kite := range queriedKites {
		key, err := kodingkey.FromString(kite.KodingKey)
		if err != nil {
			return nil, fmt.Errorf("Koding Key is invalid at Kite: %s", kite.Name)
		}

		// username is from requester, key is from kite owner
		tokenString, err := token.NewToken(requester, kite.ID).EncryptString(key)
		if err != nil {
			return nil, errors.New("Server error: Cannot generate a token")
		}

		kwt := protocol.KiteWithToken{
			Kite:  kite.Kite,
			Token: tokenString,
		}

		kitesWithToken = append(kitesWithToken, kwt)
	}

	return kitesWithToken, nil
}

// query makes the query based on the KontrolQuery struct and returns
// a list of kites that match the query.
func (k *KontrolQuery) query() []models.Kite {
	queries := k.structToMap()
	kites := make([]models.Kite, 0)

	query := func(c *mgo.Collection) error {
		iter := c.Find(queries).Iter()
		return iter.All(&kites)
	}

	mongodb.Run(modelhelper.KitesCollection, query)
	return kites
}

// structToMap converts a query into key/value map. That means
// suppose you have this query struct:
//
//		type KontrolQuery struct {
//			Name       string
//			Hostname   string
//          Pid   	   int
//			Region     string
//		}
//
// and declared like:
//
// 		query := KontrolQuery{
// 			Name:   "Arslan",
// 			Region: "Turkey",
// 		}
//
// this get converted to:
//
// 		map[string]string{"name":"Arslan", "region":"Turkey"}
//
// As you see empty and non-string typed fields are neglected. Works perfectly
// with bson.M type.
func (k *KontrolQuery) structToMap() map[string]interface{} {
	mapping := make(map[string]interface{})

	t := reflect.TypeOf(*k) // because k is a pointer to struct, we need the value
	v := reflect.ValueOf(*k)

	for i := 0; i < t.NumField(); i++ {
		fieldName := strings.ToLower(t.Field(i).Name)
		fieldValue, ok := v.Field(i).Interface().(string)
		if !ok {
			continue
		}

		if fieldValue == "" {
			continue
		}

		mapping[fieldName] = fieldValue
	}

	return mapping
}
