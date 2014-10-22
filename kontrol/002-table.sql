-- create a schema for our tables
CREATE SCHEMA kite;

-- give usage access to schema for our role
GRANT USAGE ON SCHEMA kite TO kontrol;
GRANT USAGE ON SCHEMA kite TO kontrolapplication;

-- add our schema to search path
-- with this way we can use our table name directly without the schema name.

ALTER DATABASE kontrol SET search_path="$user", public, kite;


-- create the table
CREATE TABLE "kite"."kite" (
    username TEXT NOT NULL,
    environment TEXT NOT NULL,
    kitename TEXT NOT NULL,
    version TEXT NOT NULL,
    region TEXT NOT NULL,
    hostname TEXT NOT NULL,
    id uuid PRIMARY KEY,
    url TEXT NOT NULL,
    created_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'), -- you may set a global timezone
    updated_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC')
);

-- add proper permissions for table
GRANT SELECT, INSERT, UPDATE, DELETE ON "kite"."kite" TO "kontrol";

-- create the index
DROP INDEX IF EXISTS kite_updated_at_btree_idx;

CREATE INDEX kite_updated_at_btree_idx ON "kite"."kite" USING BTREE (updated_at DESC);
