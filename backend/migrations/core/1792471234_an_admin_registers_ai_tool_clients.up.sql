-- An admin registers an AI tool's OAuth client by hand, optionally with a
-- secret, for tools that cannot register themselves (Gemini Enterprise,
-- Copilot Studio "Manual"). A row with no hash is a public client, which is
-- every row that exists before this migration, so no connection changes.
SET LOCAL lock_timeout = '5s';

ALTER TABLE oauth_client ADD COLUMN client_secret_hash text;

-- Only the admin path mints a secret: a self-registered or metadata-document
-- client stays public, proven by PKCE alone.
ALTER TABLE oauth_client ADD CONSTRAINT oauth_client_secret_is_admin_only
    CHECK (client_secret_hash IS NULL OR created_via = 'admin') NOT VALID;

-- agent_client is who may register, rotate, disable and delete those clients,
-- and turn the self-registration and metadata-document methods on or off.
-- Admin alone writes it, as with the sign-in policy; management and ops read
-- it. Every other system role is written out with zero grants, so a reader
-- cannot mistake a decision for a role the backfill missed.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,agent_client}',
        '{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'agent_client';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,agent_client}',
        '{"create":false,"read":true,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('management', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'agent_client';
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,agent_client}',
        '{"create":false,"read":false,"update":false,"delete":false}'::jsonb, true)
    WHERE is_system
      AND key IN ('manager', 'rep', 'read_only')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'agent_client';
