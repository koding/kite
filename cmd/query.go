package cmd

import (
	"flag"
	"fmt"

	"github.com/koding/kite"
	"github.com/koding/kite/config"
	"github.com/koding/kite/protocol"
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
	r.client.Config = config.MustGet()

	var query protocol.KontrolQuery

	flags := flag.NewFlagSet("query", flag.ExitOnError)
	flags.StringVar(&query.Username, "username", r.client.Kite().Username, "")
	flags.StringVar(&query.Environment, "environment", "", "")
	flags.StringVar(&query.Name, "name", "", "")
	flags.StringVar(&query.Version, "version", "", "")
	flags.StringVar(&query.Region, "region", "", "")
	flags.StringVar(&query.Hostname, "hostname", "", "")
	flags.StringVar(&query.ID, "id", "", "")
	flags.Parse(args)

	result, err := r.client.GetKites(query)
	if err != nil {
		return err
	}

	for i, client := range result {
		var k *protocol.Kite = &client.Kite
		fmt.Printf("%d\t%s/%s/%s/%s/%s/%s/%s\t%s\n", i+1, k.Username, k.Environment, k.Name, k.Version, k.Region, k.Hostname, k.ID, client.URL)
	}

	return nil
}
