-- A test_mailbox connection exists only behind a flag.
--
-- test_mailbox is the QC-only, no-network connector that can both capture
-- and send synthetic mail (issue #4974). Its capture_connection rows need a
-- provider value the CHECK admits, exactly like every other provider this
-- constraint was widened for (graphcal, 1788190060).
--
-- Widening a CHECK is additive: every existing row already satisfies it, so
-- the constraint is replaced rather than validated over history.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo', 'test_mailbox'));

-- What test_mailbox echoes back to itself: one row per SendEmail call, read
-- by the connector's own Sync to file a matching inbound copy and reconcile
-- it against the outbound activity by message_id, then swept — this is
-- connector-internal bookkeeping, not a tenant record, so (like
-- capture_trace) it carries no workspace_id and no RLS policy.
--
-- Keyed on user_id, not capture_connection_id, and deliberately with no FK to
-- capture_connection at all — matching capture_trace's own choice. The
-- connect handler (compose/connectors_testmailbox.go) knows the granting
-- human's user_id BEFORE Registry.Connect ever runs (it is the authenticated
-- actor), so there is no need to know the connection row's own (DB-generated)
-- id.
CREATE TABLE capture_test_mailbox_sent (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    message_id text NOT NULL,
    to_addresses jsonb NOT NULL,
    subject text NOT NULL,
    sent_at timestamptz DEFAULT now() NOT NULL,
    echoed_at timestamptz,
    CONSTRAINT capture_test_mailbox_sent_pkey PRIMARY KEY (id),
    CONSTRAINT capture_test_mailbox_sent_message_id_check
        CHECK (length(message_id) > 0 AND char_length(message_id) <= 512)
);

CREATE INDEX idx_capture_test_mailbox_sent_unechoed
    ON capture_test_mailbox_sent (user_id, sent_at)
    WHERE echoed_at IS NULL;
