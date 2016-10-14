-- Here is the required steps to run kontrol with postgresql storage.

-- those can be helpful for a fresh start

-- drop the database
-- DROP DATABASE IF EXISTS koding;

-- drop the role
-- DROP ROLE IF EXISTS kontrol;

-- drop the user
-- DROP USER IF EXISTS kontrol;

-- drop the tables
-- DROP TABLE IF EXISTS "kite"."kite";
-- DROP TABLE IF EXISTS "kite"."key";

-- create role
CREATE ROLE kontrol;

-- create user
-- please change this password according to your conventions
CREATE USER kontrolapplication PASSWORD 'kontrolapplication';

GRANT kontrol TO kontrolapplication;
