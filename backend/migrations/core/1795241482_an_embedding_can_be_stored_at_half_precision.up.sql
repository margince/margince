-- A vector at half precision takes half the room, and similarity search reads
-- every row of a record type. New rows are written here, and a data task moves
-- the stored ones over. Each ALTER changes the catalog only: no row is rewritten.
SET LOCAL lock_timeout = '3s';

-- The half-precision type came with pgvector 0.7.0. The extension belongs to a
-- superuser, so this role cannot update it and says who can.
DO $$
BEGIN
    IF to_regtype('halfvec') IS NULL THEN
        RAISE EXCEPTION 'pgvector 0.7.0 or newer is needed. As a superuser of this database, run: ALTER EXTENSION vector UPDATE';
    END IF;
END $$;

ALTER TABLE embedding ADD COLUMN embedding_half halfvec;
ALTER TABLE embedding ALTER COLUMN embedding DROP NOT NULL;
ALTER TABLE embedding ADD CONSTRAINT embedding_holds_one_vector
    CHECK (num_nonnulls(embedding, embedding_half) = 1) NOT VALID;
