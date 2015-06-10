-- create a schema for our tables
CREATE SCHEMA kite;

-- give usage access to schema for our role
GRANT USAGE ON SCHEMA kite TO kontrol;

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

--
-- create key table for storing key pairs
--
CREATE TABLE IF NOT EXISTS "kite"."key" (
    id UUID NOT NULL DEFAULT uuid_generate_v4(),
    public TEXT NOT NULL COLLATE "default", -- public will store public key of pair
    private TEXT NOT NULL COLLATE "default", -- private will store private key of pair
    created_at timestamp(6) WITH TIME ZONE NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    deleted_at timestamp(6) WITH TIME ZONE, -- update deleted at if a key pair become obsolote

    -- create constraints along with table creation
    PRIMARY KEY ("id") NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT "key_public_unique" UNIQUE ("public") NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT "key_private_unique" UNIQUE ("private") NOT DEFERRABLE INITIALLY IMMEDIATE,
    CONSTRAINT "key_created_at_lte_deleted_at_check" CHECK (created_at <= deleted_at)
) WITH (OIDS = FALSE);

GRANT SELECT, INSERT, UPDATE ON "kite"."key" TO "kontrol"; -- dont allow deletion from this table

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
    updated_at timestamptz NOT NULL DEFAULT (NOW() AT TIME ZONE 'UTC'),
    key_id UUID NOT NULL,

    CONSTRAINT "kite_key_id_fkey" FOREIGN KEY ("key_id") REFERENCES kite.key (id) ON UPDATE NO ACTION ON DELETE NO ACTION NOT DEFERRABLE INITIALLY IMMEDIATE
);

-- add proper permissions for table
GRANT SELECT, INSERT, UPDATE, DELETE ON "kite"."kite" TO "kontrol";

-- create the index, but drop first if exists
DROP INDEX IF EXISTS kite_updated_at_btree_idx;

CREATE INDEX kite_updated_at_btree_idx ON "kite"."kite" USING BTREE (updated_at DESC);

