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
        AND (history_baseline_date IS NULL) = (history_baseline_set_at IS NULL));

-- The connectors that delivered a message to this seat, so a per-mailbox
-- setting applies only to mail that mailbox brought in.
ALTER TABLE capture_import
    ADD COLUMN providers text[] NOT NULL DEFAULT '{}';
