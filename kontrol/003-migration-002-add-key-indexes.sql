DO $$
  BEGIN
    BEGIN
      CREATE INDEX  "kite_key_public_idx" ON "kite"."key" USING btree(public);
    EXCEPTION WHEN duplicate_table THEN
      RAISE NOTICE 'kite_key_public_idx already exists';
    END;

    BEGIN
      CREATE INDEX  "kite_key_deleted_at_public_idx" ON "kite"."key" USING btree(deleted_at DESC NULLS FIRST, public);
    EXCEPTION WHEN duplicate_table THEN
      RAISE NOTICE 'kite_key_deleted_at_public_idx already exists';
    END;

  END;
$$;
