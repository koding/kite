package kontrol

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"strings"

	_ "github.com/lib/pq"

	"github.com/koding/kite"
	kontrolprotocol "github.com/koding/kite/kontrol/protocol"
	"github.com/koding/kite/protocol"
	"github.com/koding/logging"
)

// Postgres holds Postgresql database related configuration
type PostgresConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	DBName   string
}

type Postgres struct {
	DB  *sql.DB
	Log logging.Logger
}

func NewPostgres(conf *PostgresConfig, log kite.Logger) *Postgres {
	if conf.Port == 0 {
		conf.Port = 5432
	}

	if conf.Host == "" {
		conf.Host = "localhost"
	}

	if conf.DBName == "" {
		conf.DBName = "test"
	}

	connString := fmt.Sprintf(
		"host=%s port=%d dbname=%s sslmode=disable",
		conf.Host, conf.Port, conf.DBName,
	)

	if conf.Password != "" {
		connString += " password=" + conf.Password
	}

	if conf.Username != "" {
		connString += " user=" + conf.Username
	}

	fmt.Printf("connString %+v\n", connString)

	db, err := sql.Open("postgres", connString)
	if err != nil {
		panic(err)
	}

	// enable the ltree module which we are going to use, any error means it's
	// failed so there is no sense to continue, panic!
	enableTree := `CREATE EXTENSION IF NOT EXISTS ltree`
	if _, err := db.Exec(enableTree); err != nil {
		panic(err)
	}

	// create our initial kites table
	// * kite is going to be our ltree
	// * url is cointaining the kite's register url
	// * id is going to be kites' unique id (which also exists in the ltree
	//   path). We are adding it as a primary key so each kite with the full
	//   path can only exist once.
	// * created_at and updated_at are updated at creation and updating (like
	//  if the URL has changed)
	// Some notes:
	// * path label can only contain a sequence of alphanumeric characters
	//   and underscores. So for example a version string of "1.0.4" needs to
	//   be converted to "1_0_4" or uuid of 1111-2222-3333-4444 needs to be
	//   converted to 1111_2222_3333_4444.
	table := `CREATE TABLE IF NOT EXISTS kites (
		kite ltree NOT NULL,
		url text NOT NULL,
		id uuid PRIMARY KEY,
		created_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
		updated_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
	);`

	if _, err := db.Exec(table); err != nil {
		panic(err)
	}

	return &Postgres{
		DB: db,
	}
}

func (p *Postgres) Get(query *protocol.KontrolQuery) (Kites, error) {
	return nil, errors.New("GET is not implemented")
}

// ltreeLabel satisfies a valid ltree definition of a label in path. According
// to the definition it is: "A label is a sequence of alphanumeric characters
// and underscores (for example, in C locale the characters A-Za-z0-9_ are
// allowed). Labels must be less than 256 bytes long."
// We could express one character with "[A-Za-z0-9_]", a word with
// "[A-Za-z0-9_]+". However we want to catch words that are not valid labels so
// we negate them with the "^" character, so it will be : "[^[A-Za-z0-9_]]+".
// Finally we cann use the POSIX character class: [:word:] which is:
// "Alphanumeric characters plus "_"", so the final regexp will be
// "[^[:word]]+"
var invalidLabelRe = regexp.MustCompile("[^[:word:]]+")

// ltreePath returns a query path to be used with the ltree module in postgress
// in the form of "username.environment.kitename.version.region.hostname.id"
func ltreePath(query *protocol.KontrolQuery) string {
	// username should exist because it's the first parent in the ltree path
	if query.Username == "" {
		return ""
	}

	path := ""
	fields := query.Fields()

	// we stop for the first empty value
	for _, key := range keyOrder {
		v := fields[key]
		if v == "" {
			break
		}

		// replace anything that doesn't match the definition for a ltree path
		// label with a underscore, so the version "0.0.1" will be "0_0_1", or
		// uuid of "1111-2222-3333-4444" will be converted to
		// 1111_2222_3333_4444.
		v = invalidLabelRe.ReplaceAllLiteralString(v, "_")

		path = path + v + "."
	}

	// remove the latest dot which causes an invalid query
	path = strings.TrimSuffix(path, ".")

	return path
}

func (p *Postgres) Set(kite *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	return errors.New("SET is not implemented")
}

func (p *Postgres) Delete(kite *protocol.Kite) error {
	return errors.New("DELETE is not implemented")
}
