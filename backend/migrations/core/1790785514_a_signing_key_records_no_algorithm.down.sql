SET LOCAL lock_timeout = '3s';

ALTER TABLE signing_key ADD COLUMN alg text NOT NULL DEFAULT 'EdDSA';
ALTER TABLE signing_key ADD CONSTRAINT workspace_signing_key_alg_check CHECK (alg = 'EdDSA');
