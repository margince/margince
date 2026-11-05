-- A mailbox owner may name a local date before which captured mail suggests no
-- automatic commitments. The date, zone and who set them change together.
SET LOCAL lock_timeout = '5s';

ALTER TABLE capture_connection
    ADD COLUMN history_baseline_date date,
    ADD COLUMN history_baseline_tz text,
    ADD COLUMN history_baseline_set_by text,
    ADD COLUMN history_baseline_set_at timestamptz,
    ADD CONSTRAINT capture_connection_history_baseline_check CHECK (
        (history_baseline_date IS NULL) = (history_baseline_tz IS NULL)
        AND (history_baseline_date IS NULL) = (history_baseline_set_by IS NULL)
        AND (history_baseline_date IS NULL) = (history_baseline_set_at IS NULL)) NOT VALID;

-- The mailboxes that delivered a message to this seat, as provider:generation.
-- A rebind bumps the generation, so a new account never claims the old one's mail.
ALTER TABLE capture_import
    ADD COLUMN delivered_by text[] NOT NULL DEFAULT '{}';
