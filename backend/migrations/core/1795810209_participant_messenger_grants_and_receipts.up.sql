SET LOCAL lock_timeout = '5s';

CREATE TABLE chat_member_grant (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 provider text NOT NULL CHECK (provider IN ('dispact','slack','teams')),
 provider_workspace text NOT NULL,
 provider_user text NOT NULL,
 address text NOT NULL DEFAULT '',
 credential_ref text NOT NULL,
 status text NOT NULL DEFAULT 'connected' CHECK (status IN ('connected','disconnected','reauth_required')),
 version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN 1 AND 9007199254740991),
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,provider),
 UNIQUE(provider,provider_workspace,provider_user)
);
CREATE TRIGGER trg_chat_member_grant_updated BEFORE UPDATE ON chat_member_grant
 FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

CREATE TABLE chat_send_attempt (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
 grant_id uuid NOT NULL REFERENCES chat_member_grant(id) ON DELETE RESTRICT,
 grant_version bigint NOT NULL,
 record_type text NOT NULL CHECK (record_type IN ('company','deal','project')),
 record_id uuid NOT NULL,
 conversation_id text NOT NULL,
 root_id text NOT NULL DEFAULT '',
 request_id uuid NOT NULL,
 content_hash text NOT NULL,
 audience_hash text NOT NULL,
 state text NOT NULL CHECK (state IN ('pending','accepted','failed','unknown')),
 provider_message_id text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,request_id)
);
CREATE INDEX idx_chat_send_attempt_grant ON chat_send_attempt(grant_id);
CREATE INDEX idx_chat_send_attempt_owner ON chat_send_attempt(user_id,created_at DESC);

CREATE TABLE chat_member_room (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 grant_id uuid NOT NULL REFERENCES chat_member_grant(id) ON DELETE CASCADE,
 grant_version bigint NOT NULL,
 record_type text NOT NULL CHECK (record_type IN ('company','deal','project')),
 record_id uuid NOT NULL,
 roster_hash text NOT NULL,
 conversation_id text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(grant_id,grant_version,record_type,record_id,roster_hash)
);
