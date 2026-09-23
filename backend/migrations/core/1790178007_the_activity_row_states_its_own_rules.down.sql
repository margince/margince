SET LOCAL lock_timeout = '5s';

CREATE INDEX IF NOT EXISTS idx_activity_counterparty_outbound_attested
    ON activity (counterparty_email)
    WHERE counterparty_email IS NOT NULL AND counterparty_outbound_attested;

ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_not_derived_from_itself;
ALTER TABLE activity DROP CONSTRAINT IF EXISTS activity_capture_label_stamped;

-- The column stays false-filled: NULL and false were already the same answer to
-- every reader, so restoring nullability restores the shape and not the rows.
ALTER TABLE activity
    ALTER COLUMN has_calendar_part DROP NOT NULL,
    ALTER COLUMN has_calendar_part DROP DEFAULT;
