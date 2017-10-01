package kontrol

import (
	"database/sql"
	"fmt"

	_ "github.com/herenow/go-crate"

	"github.com/koding/kite"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
	"github.com/koding/multiconfig"
)

const (
	CratePrefix = "kontrol_crate"
)

// Crate holds Crate database related configuration
type CrateConfig struct {
	Host         string `default:"127.0.0.1"`
	Port         int    `default:"4200"`
	Table        string `default:"kontrol"`
	TableCreated bool   `default:"false"`
}

type Crate struct {
	DB           *sql.DB
	Log          kite.Logger
	Table        string
	TableCreated bool
}

func NewCrate(conf *CrateConfig, log kite.Logger) *Crate {
	if conf == nil {
		conf = new(CrateConfig)

		envLoader := &multiconfig.EnvironmentLoader{Prefix: CratePrefix}
		configLoader := multiconfig.MultiLoader(
			&multiconfig.TagLoader{}, envLoader,
		)

		if err := configLoader.Load(conf); err != nil {
			log.Error("Valid environment variables are: ")
			envLoader.PrintEnvs(conf)
			log.Fatal("%v", err)
		}

		err := multiconfig.MultiValidator(&multiconfig.RequiredValidator{}).Validate(conf)
		if err != nil {
			log.Error("Valid environment variables are: ")
			envLoader.PrintEnvs(conf)
			log.Fatal("%v", err)
		}
	}

	connString := fmt.Sprintf("http://%s:%d/", conf.Host, conf.Port)

	db, err := sql.Open("crate", connString)
	if err != nil {
		log.Fatal("%v", err)
	}

	c := &Crate{
		DB:           db,
		Log:          log,
		Table:        conf.Table,
		TableCreated: conf.TableCreated,
	}

	// TODO Shouldn't this be managed by a distributed cron daemon?
	// cleanInterval := 120 * time.Second // clean every 120 second
	// go p.RunCleaner(cleanInterval, KeyTTL)

	return c
}

// exec calls Crate.Log.Debug then calls Exec.
func (c *Crate) exec(cmd string, args ...interface{}) (sql.Result, error) {
	if !c.TableCreated {
		c.Log.Debug("CrateDB.Exec running table creation: %s", cmd)
		c.TableCreated = true
		cmd := "CREATE TABLE IF NOT EXISTS " + c.Table + " ( " +
			"id string PRIMARY KEY, " +
			"name string, " +
			"username string, " +
			"environment string, " +
			"region string, " +
			"version string, " +
			"hostname string, " +
			"key_id string, " +
			"url string)"
		_, err := c.exec(cmd)
		if err != nil {
			c.Log.Fatal("%v", err)
		}
	}

	c.Log.Debug("CrateDB.Exec: %s", cmd)
	result, err := c.DB.Exec(cmd, args...)
	if err != nil {
		c.Log.Debug("CrateDB.Exec ERROR: %v", err)
	}
	return result, err
}

// Get retrieves the Kites with the given query
func (c *Crate) Get(query *protocol.KontrolQuery) (Kites, error) {
	return nil, fmt.Errorf("Not Impmentented")
	// // We will make a get request to etcd store with this key. So get a "etcd"
	// // key from the given query so that we can use it to query from Etcd.
	// etcdKey, err := e.etcdKey(query)
	// if err != nil {
	// 	return nil, err
	// }

	// // If version field contains a constraint we need no make a new query up to
	// // "name" field and filter the results after getting all versions.
	// // NewVersion returns an error if it's a constraint, like: ">= 1.0, < 1.4"
	// // Because NewConstraint doesn't return an error for version's like "0.0.1"
	// // we check it with the NewVersion function.
	// var hasVersionConstraint bool // does query contains a constraint on version?
	// var keyRest string            // query key after the version field
	// var versionConstraint version.Constraints
	// _, err = version.NewVersion(query.Version)
	// if err != nil && query.Version != "" {
	// 	// now parse our constraint
	// 	versionConstraint, err = version.NewConstraint(query.Version)
	// 	if err != nil {
	// 		// version is a malformed, just return the error
	// 		return nil, err
	// 	}

	// 	hasVersionConstraint = true
	// 	nameQuery := &protocol.KontrolQuery{
	// 		Username:    query.Username,
	// 		Environment: query.Environment,
	// 		Name:        query.Name,
	// 	}
	// 	// We will make a get request to all nodes under this name
	// 	// and filter the result later.
	// 	etcdKey, _ = GetQueryKey(nameQuery)

	// 	// Rest of the key after version field
	// 	keyRest = "/" + strings.TrimRight(
	// 		query.Region+"/"+query.Hostname+"/"+query.ID, "/")
	// }

	// resp, err := e.client.Get(context.TODO(),
	// 	KitesPrefix+"/"+etcdKey,
	// 	&etcd.GetOptions{
	// 		Recursive: true,
	// 		Sort:      false,
	// 	},
	// )
	// if err != nil {
	// 	return nil, err
	// }

	// kites := make(Kites, 0)
	// node := NewNode(resp.Node)

	// // means a query with all fields were made or a query with an ID was made,
	// // in which case also returns a full path. This path has a value that
	// // contains the final kite URL. Therefore this is a single kite result,
	// // create it and pass it back.
	// if node.HasValue() {
	// 	oneKite, err := node.Kite()
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	kites = append(kites, oneKite)
	// } else {
	// 	kites, err = node.Kites()
	// 	if err != nil {
	// 		return nil, err
	// 	}

	// 	// Filter kites by version constraint
	// 	if hasVersionConstraint {
	// 		kites.Filter(versionConstraint, keyRest)
	// 	}
	// }

	// // Shuffle the list
	// kites.Shuffle()

	// return kites, nil
}

// Add inserts the given kite with the given value
func (c *Crate) Add(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	cmd := "INSERT INTO " + c.Table + " (id, name, username, environment, " +
		"region, version, hostname, key_id, url) " +
		"VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?) " +
		"on duplicate key update " +
		"name=VALUES(name), username=VALUES(username), " +
		"environment=VALUES(environment), region=VALUES(region), " +
		"version=VALUES(version), hostname=VALUES(hostname), " +
		"key_id=VALUES(key_id), url=VALUES(url)"
	_, err := c.exec(
		cmd,
		kite.ID,
		kite.Name,
		kite.Username,
		kite.Environment,
		kite.Region,
		kite.Version,
		kite.Hostname,
		value.KeyID,
		value.URL,
	)
	return err
}

// Update updates the value for the given kite
func (c *Crate) Update(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	return c.Add(kite, value)
}

// Delete deletes the given kite from the storage
func (c *Crate) Delete(kite *protocol.Kite) error {
	_, err := c.exec(
		"DELETE FROM "+c.Table+" WHERE ($1, $2)",
		"gopher",
		27,
	)
	return err
}

// Upsert inserts or updates the value for the given kite
func (c *Crate) Upsert(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	return c.Add(kite, value)
}
