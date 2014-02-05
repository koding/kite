package cmd

import (
	"flag"
	"fmt"
	"kite"
	"kite/cmd/util"
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
	token, err := util.ParseKiteKey()
	if err != nil {
		return err
	}

	username, _ := token.Claims["sub"].(string)

	var query protocol.KontrolQuery
	flags := flag.NewFlagSet("query", flag.ContinueOnError)
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

	target := protocol.Kite{URL: protocol.KiteURL{parsed}}
	regserv := r.client.NewRemoteKite(target, kite.Authentication{})
	if err = regserv.Dial(); err != nil {
		return err
	}

	result, err := regserv.Tell("getKites", query)
	if err != nil {
		return err
	}

	fmt.Println(result)
	return nil
}
