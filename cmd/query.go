package cmd

import (
	"flag"
	"fmt"
	"kite"
	"kite/kitekey"
	"kite/protocol"
	"net/url"
)

type Query struct {
	client *kite.Kite
}

func NewQuery(client *kite.Kite) *Query {
	return &Query{
		client: client,
	}
}

func (r *Query) Definition() string {
	return "Query kontrol"
}

func (r *Query) Exec(args []string) error {
	token, err := kitekey.Parse()
	if err != nil {
		return err
	}

	username, _ := token.Claims["sub"].(string)

	var query protocol.KontrolQuery
	flags := flag.NewFlagSet("query", flag.ExitOnError)
	flags.StringVar(&query.Username, "username", username, "")
	flags.StringVar(&query.Environment, "environment", "", "")
	flags.StringVar(&query.Name, "name", "", "")
	flags.StringVar(&query.Version, "version", "", "")
	flags.StringVar(&query.Region, "region", "", "")
	flags.StringVar(&query.Hostname, "hostname", "", "")
	flags.StringVar(&query.ID, "id", "", "")
	flags.Parse(args)

	parsed, err := url.Parse(token.Claims["kontrolURL"].(string))
	if err != nil {
		return err
	}

	kontrol := r.client.NewKontrol(parsed)
	if err = kontrol.Dial(); err != nil {
		return err
	}

	response, err := kontrol.Tell("getKites", query)
	if err != nil {
		return err
	}

	var kites []protocol.KiteWithToken
	err = response.Unmarshal(&kites)
	if err != nil {
		return err
	}

	for i, kite := range kites {
		var k *protocol.Kite = &kite.Kite
		fmt.Printf("%d\t%s/%s/%s/%s/%s/%s/%s\t%s\n", i+1, k.Username, k.Environment, k.Name, k.Version, k.Region, k.Hostname, k.ID, k.URL)
	}

	return nil
}
