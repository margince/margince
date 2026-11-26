SET LOCAL lock_timeout = '3s';

ALTER TABLE raw_capture
    DROP COLUMN IF EXISTS ask_followup_at,
    DROP COLUMN IF EXISTS ask_next_at,
    DROP COLUMN IF EXISTS ask_attempts,
    DROP COLUMN IF EXISTS ask_sent_at,
    DROP COLUMN IF EXISTS ask_claimed_at,
    DROP COLUMN IF EXISTS ask_message_ids,
    DROP COLUMN IF EXISTS ask_batch_id,
    DROP COLUMN IF EXISTS held_connection,
    DROP COLUMN IF EXISTS held_by;
