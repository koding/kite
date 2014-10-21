-- Here is the required steps to run kontrol with postgresql storage.

-- those can be helpful for a fresh start

-- drop the database
-- DROP DATABASE IF EXISTS koding;

-- drop the role
-- DROP ROLE IF EXISTS kontrol;

-- drop the user
-- DROP USER IF EXISTS kontrol;

-- drop the table
-- DROP TABLE IF EXISTS kite.kite;

-- create role
CREATE ROLE kontrol;

-- create user
-- please change this password according to your conventions
CREATE USER kontrolapplication PASSWORD 'somerandompassword';

-- make the user a member of the role
GRANT kontrol TO kontrolapplication;

-- create a schema for our tables
CREATE SCHEMA kite;

-- give usage access to schema for our role
GRANT USAGE ON SCHEMA kite TO kontrol;

-- add our schema to search path
-- with this way we can use our table name directly without the schema name.
SELECT
	set_config (
		'search_path',
		current_setting ('search_path') || ',kite',
		FALSE
	);

-- create the table
CREATE TABLE kite (
	username TEXT NOT NULL,
	environment TEXT NOT NULL,
	kitename TEXT NOT NULL,
	version TEXT NOT NULL,
	region TEXT NOT NULL,
	hostname TEXT NOT NULL,
	ID uuid PRIMARY KEY,
	url TEXT NOT NULL,
	created_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'), -- you may set a global timezone
	updated_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

-- create the index
DROP INDEX IF EXISTS kite.kite_updated_at_btree_idx;

CREATE INDEX kite.kite_updated_at_btree_idx ON kite.kite USING BTREE (updated_at DESC);

