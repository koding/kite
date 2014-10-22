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
CREATE USER kontrolapplication PASSWORD 'somerandompassword';

GRANT kontrol TO kontrolapplication;

CREATE DATABASE kontrol OWNER kontrol;

-- create a schema for our tables
CREATE SCHEMA kite;

-- give usage access to schema for our role
GRANT USAGE ON SCHEMA kite TO kontrol;
GRANT USAGE ON SCHEMA kite TO kontrolapplication;

-- add our schema to search path
-- with this way we can use our table name directly without the schema name.

ALTER DATABASE kontrol SET search_path="$user", public, kite;
