package kontrol

import (
	"database/sql"
	"fmt"
	"strings"
	"time"

	_ "github.com/herenow/go-crate"

	"github.com/koding/kite"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/kontrol/util"
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

// Wait calls DB.Ping until the timeout is reached.
func (c *Crate) Wait(timeout time.Duration) error {
	return util.PingTimeout(c.DB, timeout)
}

// exec calls Crate.Log.Debug then calls Exec.
func (c *Crate) exec(cmd string, args ...interface{}) (sql.Result, error) {
	if !c.TableCreated {
		c.Log.Debug("CrateDB.Exec running table creation: %s", cmd)
		c.createTable()
	}

	c.Log.Debug("CrateDB.Exec: %s", cmd)
	result, err := c.DB.Exec(cmd, args...)
	if err != nil {
		c.Log.Debug("CrateDB.Exec ERROR: %v", err)
	}
	return result, err
}

func (c *Crate) createTable() error {
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
	return err
}

func (c *Crate) whereArgs(query *protocol.KontrolQuery) (string, []interface{}) {
	where := make([]string, 0)
	args := make([]interface{}, 0)
	for k, v := range query.Fields() {
		if len(v) > 0 {
			where = append(where, fmt.Sprintf("%s = ?", k))
			args = append(args, v)
		}
	}
	return strings.Join(where, ", "), args
}

// Get retrieves the Kites with the given query
func (c *Crate) Get(query *protocol.KontrolQuery) (Kites, error) {
	kites := make(Kites, 0)
	where, args := c.whereArgs(query)
	cmd := "SELECT id, name, username, environment, region, version, " +
		"hostname, key_id, url FROM " + c.Table + " WHERE " + where
	c.Log.Debug("CrateDB.Get: %s", cmd)
	rows, err := c.DB.Query(cmd, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		kite := protocol.KiteWithToken{}
		err := rows.Scan(
			&kite.Kite.ID,
			&kite.Kite.Name,
			&kite.Kite.Username,
			&kite.Kite.Environment,
			&kite.Kite.Region,
			&kite.Kite.Version,
			&kite.Kite.Hostname,
			&kite.KeyID,
			&kite.URL,
		)
		if err != nil {
			return nil, err
		}
		kites = append(kites, &kite)
	}
	err = rows.Err()
	if err != nil {
		return nil, err
	}

	return kites, nil
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
	where, args := c.whereArgs(kite.Query())
	_, err := c.exec("DELETE FROM "+c.Table+" WHERE "+where, args...)
	return err
}

// Upsert inserts or updates the value for the given kite
func (c *Crate) Upsert(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	return c.Add(kite, value)
}
