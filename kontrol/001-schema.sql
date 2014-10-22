-- Here is the required steps to run kontrol with postgresql storage.

-- those can be helpful for a fresh start

-- drop the database
-- DROP DATABASE IF EXISTS koding;

-- drop the role
-- DROP ROLE IF EXISTS kontrol;

-- drop the user
-- DROP USER IF EXISTS kontrol;

-- drop the table
-- DROP TABLE IF EXISTS "kite"."kite";

-- create role
CREATE ROLE kontrol;

-- create user
-- please change this password according to your conventions
CREATE USER kontrolapp WITH PASSWORD 'kontrolapp';

-- make the user a member of the role
GRANT kontrol TO kontrolapp;

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
