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

-- add key_id column into kite table
DO $$
  BEGIN
    BEGIN
      ALTER TABLE kite.kite ADD COLUMN "key_id" UUID NOT NULL;
    EXCEPTION
      WHEN duplicate_column THEN RAISE NOTICE 'key_id column already exists';
    END;
  END;
$$;

-- create foreign constraint between kite.kite.key_id and kite.key.id
DO $$
  BEGIN
    BEGIN
      ALTER TABLE kite.kite ADD CONSTRAINT "kite_key_id_fkey" FOREIGN KEY ("key_id") REFERENCES kite.key (id) ON UPDATE NO ACTION ON DELETE NO ACTION NOT DEFERRABLE INITIALLY IMMEDIATE;
    EXCEPTION
      WHEN duplicate_object THEN RAISE NOTICE 'kite_key_id_fkey already exists';
    END;
  END;
$$;

