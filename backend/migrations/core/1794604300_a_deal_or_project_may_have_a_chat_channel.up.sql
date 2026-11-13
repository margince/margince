-- A deal or a project may be linked to one channel of a team chat (Dispact),
-- and the chat's bot is connected once per installation, beside Telegram's.
--
-- channel_connection takes the Dispact bot: its provider list widens, and it
-- gains the bot's web address (Dispact is self-hosted, so every install has its
-- own) and an optional sealed secret for signed event notices.
--
-- chat_link is one record's channel: the card Margince keeps in it, how much it
-- posts, and how far the channel's history has been read. Episodes of talk are
-- not stored: the cursor marks the end of what was judged, and a filed summary
-- is an activity keyed on its episode, so a replay files nothing.
--
-- chat_post_item is one event bound for a channel. (link_id, event_id) is the
-- effect key, so a replayed event adds nothing. A flush sends the pending items
-- as one post and stamps them with its id and the chat's message id, which is
-- what lets a later narrowing rewrite that post. No chat or activity text is
-- kept here: a post is rendered from the live records when it is sent.
SET LOCAL lock_timeout = '5s';

ALTER TABLE channel_connection DROP CONSTRAINT channel_connection_provider_check;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_provider_check
    CHECK (provider IN ('telegram', 'dispact'));
ALTER TABLE channel_connection ADD COLUMN address text;
ALTER TABLE channel_connection ADD COLUMN events_credential_ref text;
ALTER TABLE channel_connection ADD CONSTRAINT channel_connection_address_check
    CHECK ((provider = 'dispact') = (address IS NOT NULL) AND (address IS NULL OR address ~ '^https?://'));

INSERT INTO channel_provider (provider, transport, label, credential_model, supplies_transport)
VALUES ('dispact', 'core', 'Dispact', 'workspace_bot', false)
ON CONFLICT DO NOTHING;

CREATE TABLE chat_link (
    id uuid DEFAULT uuidv7() NOT NULL,
    connection_id uuid NOT NULL,
    provider text NOT NULL,
    channel_slug text NOT NULL,
    channel_name text NOT NULL,
    channel_external_key text,
    deal_id uuid,
    project_id uuid,
    verbosity text DEFAULT 'important' NOT NULL,
    card_message_id text,
    card_fingerprint text,
    history_cursor bigint,
    history_synced_at timestamptz,
    backfill_requested boolean DEFAULT false NOT NULL,
    status text DEFAULT 'active' NOT NULL,
    linked_by uuid NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT chat_link_pkey PRIMARY KEY (id),
    CONSTRAINT chat_link_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES channel_connection(id) ON DELETE RESTRICT,
    CONSTRAINT chat_link_deal_id_fkey FOREIGN KEY (deal_id) REFERENCES deal(id) ON DELETE CASCADE,
    CONSTRAINT chat_link_project_id_fkey FOREIGN KEY (project_id) REFERENCES project(id) ON DELETE CASCADE,
    CONSTRAINT chat_link_linked_by_fkey FOREIGN KEY (linked_by) REFERENCES app_user(id) ON DELETE RESTRICT,
    CONSTRAINT chat_link_provider_check CHECK (provider = 'dispact'),
    CONSTRAINT chat_link_record_check CHECK (deal_id IS NOT NULL OR project_id IS NOT NULL),
    CONSTRAINT chat_link_verbosity_check CHECK (verbosity IN ('everything', 'important', 'milestones', 'silent')),
    CONSTRAINT chat_link_status_check CHECK (status IN ('active', 'channel_gone')),
    CONSTRAINT chat_link_version_js_safe CHECK (version >= '-9007199254740991'::bigint AND version <= '9007199254740991'::bigint),
    CONSTRAINT chat_link_history_cursor_js_safe CHECK (history_cursor IS NULL OR (history_cursor >= 0 AND history_cursor <= '9007199254740991'::bigint))
);

CREATE UNIQUE INDEX uq_chat_link_channel ON chat_link (provider, channel_slug) WHERE archived_at IS NULL;
CREATE UNIQUE INDEX uq_chat_link_deal ON chat_link (deal_id) WHERE archived_at IS NULL AND deal_id IS NOT NULL;
CREATE UNIQUE INDEX uq_chat_link_project ON chat_link (project_id) WHERE archived_at IS NULL AND project_id IS NOT NULL;

-- The sync columns move every minute, and a version bump on each would make
-- every edit of the link's settings lose to the pull.
CREATE TRIGGER trg_chat_link_updated BEFORE UPDATE ON chat_link FOR EACH ROW
    WHEN ((to_jsonb(OLD.*) - ARRAY['history_cursor', 'history_synced_at', 'card_message_id', 'card_fingerprint'])
          IS DISTINCT FROM (to_jsonb(NEW.*) - ARRAY['history_cursor', 'history_synced_at', 'card_message_id', 'card_fingerprint']))
    EXECUTE FUNCTION set_updated_at_bump_version();

CREATE TABLE chat_post_item (
    id uuid DEFAULT uuidv7() NOT NULL,
    link_id uuid NOT NULL,
    event_id uuid NOT NULL,
    event_type text NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    activity_id uuid,
    tier text NOT NULL,
    detail jsonb DEFAULT '{}'::jsonb NOT NULL,
    post_id uuid,
    message_id text,
    attempts integer DEFAULT 0 NOT NULL,
    last_error text,
    posted_at timestamptz,
    withdrawn_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT chat_post_item_pkey PRIMARY KEY (id),
    CONSTRAINT chat_post_item_link_id_fkey FOREIGN KEY (link_id) REFERENCES chat_link(id) ON DELETE CASCADE,
    CONSTRAINT chat_post_item_tier_check CHECK (tier IN ('milestone', 'important', 'everything')),
    CONSTRAINT chat_post_item_attempts_check CHECK (attempts >= 0),
    CONSTRAINT chat_post_item_posted_check CHECK (posted_at IS NULL OR post_id IS NOT NULL)
);

CREATE UNIQUE INDEX uq_chat_post_item_event ON chat_post_item (link_id, event_id);
CREATE UNIQUE INDEX uq_chat_post_item_activity ON chat_post_item (link_id, activity_id) WHERE activity_id IS NOT NULL;
CREATE INDEX idx_chat_post_item_pending ON chat_post_item (link_id, created_at) WHERE posted_at IS NULL;
CREATE INDEX idx_chat_post_item_activity ON chat_post_item (activity_id) WHERE activity_id IS NOT NULL;

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE chat_link TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE chat_post_item TO margince_app;

CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."channel_connection" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"address":500,"channel_id":20000,"channel_label":120,"credential_ref":20000,"events_credential_ref":20000,"provider":20000,"status":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"card_fingerprint":120,"card_message_id":500,"channel_external_key":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_external_key":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_link" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"card_fingerprint":120,"card_message_id":500,"channel_external_key":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"card_fingerprint":120,"card_message_id":500,"channel_external_key":500,"channel_name":500,"channel_slug":500,"provider":20000,"status":20000,"verbosity":20000}');
CREATE OR REPLACE TRIGGER "text_bound_insert" AFTER INSERT ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), NULL, '{"entity_type":20000,"event_type":20000,"last_error":500,"message_id":500,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"event_type":20000,"last_error":500,"message_id":500,"tier":20000}');
CREATE OR REPLACE TRIGGER "text_bound_update" AFTER UPDATE ON "public"."chat_post_item" FOR EACH ROW
WHEN (public.text_row_exceeds_bounds(to_jsonb(NEW), to_jsonb(OLD), '{"entity_type":20000,"event_type":20000,"last_error":500,"message_id":500,"tier":20000}')) EXECUTE FUNCTION public.refuse_oversized_text('{"entity_type":20000,"event_type":20000,"last_error":500,"message_id":500,"tier":20000}');
