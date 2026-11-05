-- Remove admin-registered secrets and the agent_client object. Rows an admin
-- registered stay, as public clients with no secret.
SET LOCAL lock_timeout = '5s';

UPDATE role SET permissions = permissions #- '{objects,agent_client}'
    WHERE is_system AND permissions ? 'objects';

ALTER TABLE oauth_client DROP CONSTRAINT oauth_client_secret_is_admin_only;
ALTER TABLE oauth_client DROP COLUMN client_secret_hash;
