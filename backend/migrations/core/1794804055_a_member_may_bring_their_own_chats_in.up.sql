-- A member brings their own chats into Margince through a client they run.
--
-- personal_channel_connection is the member's switch: off until they turn it on,
-- and when their client last picked replies up.
-- personal_channel_reply is a reply waiting for that client: its body is kept only
-- until the client acks it or it expires, and erasure clears the recipient.
-- personal_channel_upload is a file staged for a message that has not landed yet.
SET LOCAL lock_timeout = '5s';

CREATE TABLE personal_channel_connection (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    enabled boolean DEFAULT false NOT NULL,
    last_pull_at timestamptz,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT personal_channel_connection_pkey PRIMARY KEY (id),
    CONSTRAINT personal_channel_connection_user_key UNIQUE (user_id),
    CONSTRAINT personal_channel_connection_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE RESTRICT,
    CONSTRAINT personal_channel_connection_version_check CHECK (version BETWEEN 1 AND 9007199254740991)
);

CREATE TABLE personal_channel_reply (
    id uuid DEFAULT uuidv7() NOT NULL,
    connection_id uuid NOT NULL,
    activity_id uuid NOT NULL,
    delivery_key text NOT NULL,
    network text NOT NULL,
    thread_id text NOT NULL,
    recipient_account text,
    reply_to text,
    body text,
    state text DEFAULT 'waiting' NOT NULL,
    attempt integer DEFAULT 0 NOT NULL,
    leased_until timestamptz,
    reason text,
    provider_message_id text,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    settled_at timestamptz,
    CONSTRAINT personal_channel_reply_pkey PRIMARY KEY (id),
    CONSTRAINT personal_channel_reply_delivery_key UNIQUE (delivery_key),
    CONSTRAINT personal_channel_reply_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES personal_channel_connection(id) ON DELETE CASCADE,
    CONSTRAINT personal_channel_reply_activity_id_fkey FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT personal_channel_reply_state_check CHECK (state IN ('waiting', 'pulled', 'sent', 'failed', 'expired')),
    CONSTRAINT personal_channel_reply_settled_check CHECK ((state IN ('sent', 'failed', 'expired')) = (settled_at IS NOT NULL))
);
CREATE INDEX personal_channel_reply_open_idx ON personal_channel_reply (connection_id, created_at)
    WHERE state IN ('waiting', 'pulled');

CREATE TABLE personal_channel_upload (
    id uuid DEFAULT uuidv7() NOT NULL,
    connection_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    byte_size bigint NOT NULL,
    sha256 text NOT NULL,
    object_key text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT personal_channel_upload_pkey PRIMARY KEY (id),
    CONSTRAINT personal_channel_upload_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES personal_channel_connection(id) ON DELETE CASCADE,
    CONSTRAINT personal_channel_upload_size_check CHECK (byte_size BETWEEN 1 AND 9007199254740991)
);
CREATE INDEX personal_channel_upload_age_idx ON personal_channel_upload (created_at);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE personal_channel_connection TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE personal_channel_reply TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE personal_channel_upload TO margince_app;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."personal_channel_reply" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"delivery_key":500,"network":120,"provider_message_id":500,"reason":500,"recipient_account":500,"reply_to":500,"state":20000,"thread_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivery_key":500,"network":120,"provider_message_id":500,"reason":500,"recipient_account":500,"reply_to":500,"state":20000,"thread_id":500}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."personal_channel_reply" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"delivery_key":500,"network":120,"provider_message_id":500,"reason":500,"recipient_account":500,"reply_to":500,"state":20000,"thread_id":500}')) EXECUTE FUNCTION public.refuse_oversized_text('{"delivery_key":500,"network":120,"provider_message_id":500,"reason":500,"recipient_account":500,"reply_to":500,"state":20000,"thread_id":500}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."personal_channel_upload" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"content_type":500,"filename":500,"object_key":500,"sha256":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_type":500,"filename":500,"object_key":500,"sha256":120}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."personal_channel_upload" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"content_type":500,"filename":500,"object_key":500,"sha256":120}')) EXECUTE FUNCTION public.refuse_oversized_text('{"content_type":500,"filename":500,"object_key":500,"sha256":120}');

-- seedSystemRoles writes each role once, so installations that exist already get
-- the grants the compiled defaults carry here.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,personal_channel}',
        '{"create": true, "read": true, "update": true, "delete": false}'::jsonb, true)
    WHERE key IN ('admin', 'management', 'manager', 'rep')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'personal_channel';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,personal_channel}',
        '{"create": false, "read": false, "update": false, "delete": false}'::jsonb, true)
    WHERE key IN ('read_only', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'personal_channel';
