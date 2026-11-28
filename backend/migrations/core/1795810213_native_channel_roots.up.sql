SET LOCAL lock_timeout = '3s';

ALTER TABLE channel_connection DROP CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check
 CHECK (provider IN ('telegram','telegram_business','dispact','whatsapp','zalo_oa','slack','teams')) NOT VALID;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_status_check;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_status_check CHECK (status IN ('active','channel_gone','identity_needs_review')) NOT VALID;
ALTER TABLE chat_link DROP CONSTRAINT chat_link_provider_check;
ALTER TABLE chat_link ADD CONSTRAINT chat_link_provider_check CHECK (provider IN ('dispact','slack','teams')) NOT VALID;
ALTER TABLE chat_link ADD COLUMN root_id text;
ALTER TABLE chat_link ADD COLUMN card_paused boolean NOT NULL DEFAULT false;
ALTER TABLE chat_link ADD COLUMN card_generation uuid;
ALTER TABLE chat_link ADD COLUMN native_identity boolean NOT NULL DEFAULT false;
ALTER TABLE chat_link ADD COLUMN post_supported boolean NOT NULL DEFAULT true;
ALTER TABLE chat_link ADD COLUMN include_history boolean NOT NULL DEFAULT false;
ALTER TABLE chat_link ADD COLUMN native_cursor text;
ALTER TABLE chat_link ADD COLUMN native_scan_started_at timestamptz;
ALTER TABLE chat_link ADD COLUMN native_coverage_at timestamptz;
ALTER TABLE chat_link ADD COLUMN native_scan_error text;
ALTER TABLE chat_link ADD COLUMN last_post_error text;
ALTER TABLE chat_link ADD COLUMN native_refresh_at timestamptz;

ALTER TABLE chat_post_item ADD COLUMN last_error text;
ALTER TABLE chat_post_item ADD COLUMN redacted_at timestamptz;
ALTER TABLE chat_post_item ADD COLUMN retry_at timestamptz;

CREATE TABLE chat_root (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 link_id uuid NOT NULL REFERENCES chat_link(id) ON DELETE CASCADE,
 root_id text NOT NULL CHECK (length(root_id) BETWEEN 1 AND 1024),
 revision text NOT NULL CHECK (length(revision) BETWEEN 1 AND 128),
 applied_revision text,
 content_digest text,
 notice_revision text,
 last_error text,
 retry_at timestamptz,
 activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,
 source_deleted boolean NOT NULL DEFAULT false,
 first_seen_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(link_id,root_id)
);
CREATE INDEX chat_root_activity_idx ON chat_root(activity_id) WHERE activity_id IS NOT NULL;

INSERT INTO channel_provider(provider,transport,label,credential_model,supplies_transport)
 VALUES ('slack','core','Slack','workspace_bot',false),('teams','core','Teams','workspace_bot',false)
 ON CONFLICT DO NOTHING;

DROP TRIGGER trg_chat_link_updated ON chat_link;
CREATE TRIGGER trg_chat_link_updated BEFORE UPDATE ON chat_link FOR EACH ROW
 WHEN ((to_jsonb(OLD.*) - ARRAY['history_cursor','history_synced_at','card_message_id','card_fingerprint','native_cursor','native_scan_started_at','native_coverage_at','native_scan_error','last_post_error','native_refresh_at'])
 IS DISTINCT FROM (to_jsonb(NEW.*) - ARRAY['history_cursor','history_synced_at','card_message_id','card_fingerprint','native_cursor','native_scan_started_at','native_coverage_at','native_scan_error','last_post_error','native_refresh_at']))
 EXECUTE FUNCTION set_updated_at_bump_version();

CREATE TABLE chat_bot_delivery (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 link_id uuid NOT NULL REFERENCES chat_link(id) ON DELETE CASCADE,
 request_key text NOT NULL CHECK (length(request_key) BETWEEN 1 AND 200),
 content_hash text NOT NULL,
 audience_hash text NOT NULL,
 carried_item_ids uuid[] NOT NULL DEFAULT '{}',
 state text NOT NULL CHECK (state IN ('sending','sent','unknown','refused','reviewed_unknown')),
 message_id text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(link_id,request_key)
);
CREATE INDEX chat_bot_delivery_unconfirmed_idx ON chat_bot_delivery(link_id) WHERE state IN ('sending','unknown');

CREATE TABLE chat_team_installation (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 app_id text NOT NULL,
 tenant_id text NOT NULL,
 team_id text NOT NULL,
 native_team_id text NOT NULL,
 bot_member_id text NOT NULL,
 service_url text NOT NULL,
 team_name text NOT NULL,
 installed boolean NOT NULL,
 signed_at timestamptz NOT NULL,
 observed_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(app_id,tenant_id,team_id)
);
CREATE TABLE chat_team_thread (
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 installation_id uuid NOT NULL REFERENCES chat_team_installation(id) ON DELETE CASCADE,
 channel_id text NOT NULL,
 root_id text NOT NULL,
 conversation_id text NOT NULL,
 signed_at timestamptz NOT NULL,
 observed_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(installation_id,channel_id,root_id)
);

CREATE TABLE chat_channel_watch (
 retired_at timestamptz,
 removal_verified boolean NOT NULL DEFAULT false,
 id uuid PRIMARY KEY DEFAULT uuidv7(),
 connection_id uuid NOT NULL REFERENCES channel_connection(id) ON DELETE CASCADE,
 channel_id text NOT NULL,
 app_id text NOT NULL,
 tenant_id text NOT NULL,
 notification_url text NOT NULL,
 credential_ref text,
 subscription_id text,
 expires_at timestamptz CHECK (expires_at > created_at),
 lease_until timestamptz,
 reauthorize boolean NOT NULL DEFAULT false,
 needs_rescan boolean NOT NULL DEFAULT false,
 version bigint NOT NULL DEFAULT 1 CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
 last_error text,
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(connection_id,channel_id)
);
CREATE UNIQUE INDEX chat_channel_watch_subscription_idx ON chat_channel_watch(subscription_id) WHERE subscription_id IS NOT NULL;
