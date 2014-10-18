package kontrol

import (
	"database/sql"
	"errors"
	"fmt"

	_ "github.com/lib/pq"

	"github.com/koding/kite"
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
	connString := fmt.Sprintf(
		"host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
		conf.Host, conf.Port, conf.Username, conf.Password, conf.DBName,
	)

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
	// *  path label can only contain a sequence of alphanumeric characters
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

func (p *Postgres) Set(key, value string) error {
	return errors.New("SET is not implemented")
}

func (p *Postgres) Update(key, value string) error {
	return nil
}

func (p *Postgres) Delete(key string) error {
	return errors.New("DELETE is not implemented")
}
