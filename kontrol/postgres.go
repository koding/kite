package kontrol

import (
	"database/sql"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/hashicorp/go-version"
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

	db, err := sql.Open("postgres", connString)
	if err != nil {
		panic(err)
	}

	// add a limit so we don't hit a "too many open connections" errors. We
	// might change this in the future to tweak according to the machine and
	// usage behaviour
	db.SetMaxIdleConns(100)
	db.SetMaxOpenConns(100)

	// enable the ltree module which we are going to use, any error means it's
	// failed so there is no sense to continue, panic!
	enableTree := `CREATE EXTENSION IF NOT EXISTS ltree`
	if _, err := db.Exec(enableTree); err != nil {
		panic(err)
	}

	// create our initial kites table
	// * kite is going to be our ltree
	// * url is containing the kite's register url
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

	// We enable index on the kite field. We don't return on errors because the
	// operator `IF NOT EXISTS` doesn't work for index creation, therefore we
	// assume the indexes might be already created.
	enableGistIndex := `CREATE INDEX kite_path_gist_idx ON kites USING GIST(kite)`
	enableBtreeIndex := `CREATE INDEX kite_path_btree_idx ON kites USING BTREE(kite)`

	if _, err := db.Exec(enableGistIndex); err != nil {
		log.Warning("postgres: enable gist index: ", err)
	}

	if _, err := db.Exec(enableBtreeIndex); err != nil {
		log.Warning("postgres: enable btree index: ", err)
	}

	return &Postgres{
		DB: db,
	}
}

func (p *Postgres) Get(query *protocol.KontrolQuery) (Kites, error) {
	// only let query with usernames, otherwise the whole tree will be fetched
	// which is not good for us
	if query.Username == "" {
		return nil, errors.New("username is not specified in query")
	}

	// NewVersion returns an error if it's a constraint, like: ">= 1.0, < 1.4"
	_, err := version.NewVersion(query.Version)
	if err != nil && query.Version != "" {
		return nil, errors.New("version constraint is not implemented")
	}

	path := ltreePath(query)

	rows, err := p.DB.Query(`SELECT kite, url FROM kites WHERE kite <@ $1`, path)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var kitePath string
	var url string

	kites := make(Kites, 0)

	for rows.Next() {
		err := rows.Scan(&kitePath, &url)
		if err != nil {
			return nil, err
		}

		kiteProt, err := kiteFromPath(kitePath)
		if err != nil {
			return nil, err
		}

		kites = append(kites, &protocol.KiteWithToken{
			Kite: *kiteProt,
			URL:  url,
		})
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// randomize the result
	kites.Shuffle()

	return kites, nil
}

func (p *Postgres) Add(kiteProt *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	// check that the incoming URL is valid to prevent malformed input
	_, err := url.Parse(value.URL)
	if err != nil {
		return err
	}

	_, err = p.DB.Exec("INSERT into kites(kite, url, id) VALUES($1, $2, $3)",
		ltreePath(kiteProt.Query()),
		value.URL,
		kiteProt.ID,
	)
	return err
}

func (p *Postgres) Update(kiteProt *protocol.Kite, value *kontrolprotocol.RegisterValue) error {
	// check that the incoming url is valid to prevent malformed input
	_, err := url.Parse(value.URL)
	if err != nil {
		return err
	}

	// TODO: also consider just using WHERE id = kiteProt.ID, see how it's
	// performs out
	_, err = p.DB.Exec(`UPDATE kites SET url = $1, updated_at = (now() at time zone 'utc') 
	WHERE kite ~ $2`,
		value.URL, ltreePath(kiteProt.Query()))

	return err
}

func (p *Postgres) Delete(kite *protocol.Kite) error {
	return errors.New("DELETE is not implemented")
}

// ltreeLabel satisfies a invalid ltree definition of a label in path.
// According to the definition it is: "A label is a sequence of alphanumeric
// characters and underscores (for example, in C locale the characters
// A-Za-z0-9_ are allowed). Labels must be less than 256 bytes long."
//
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
		// 1111_2222_3333_4444. Strings that satisfies the requirement are
		// untouched.
		v = invalidLabelRe.ReplaceAllLiteralString(v, "_")

		path = path + v + "."
	}

	// remove the latest dot which causes an invalid query
	path = strings.TrimSuffix(path, ".")
	return path
}

// kiteFromPath returns a Query from the given ltree path label
func kiteFromPath(path string) (*protocol.Kite, error) {
	fields := strings.Split(path, ".")

	if len(fields) != 7 {
		return nil, fmt.Errorf("invalid ltree path: %s", path)
	}

	// those labels were converted by us, therefore convert them back
	version := strings.Replace(fields[3], "_", ".", -1)
	id := strings.Replace(fields[6], "_", "-", -1)

	return &protocol.Kite{
		Username:    fields[0],
		Environment: fields[1],
		Name:        fields[2],
		Version:     version,
		Region:      fields[4],
		Hostname:    fields[5],
		ID:          id,
	}, nil

}
