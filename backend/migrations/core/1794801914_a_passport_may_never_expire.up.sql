-- A passport minted by hand may never expire; a null expiry means exactly that.
ALTER TABLE passport ALTER COLUMN expires_at DROP NOT NULL;
