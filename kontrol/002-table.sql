-- create a schema for our tables
CREATE SCHEMA kite;

-- give usage access to schema for our role
GRANT USAGE ON SCHEMA kite TO kontrol;

-- create the table
CREATE UNLOGGED TABLE "kite"."kite" (
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
