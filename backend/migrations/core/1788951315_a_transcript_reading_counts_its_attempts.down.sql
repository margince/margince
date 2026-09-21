SET LOCAL lock_timeout = '3s';

ALTER TABLE transcript_read
    DROP CONSTRAINT transcript_read_attempt_positive;

ALTER TABLE transcript_read
    DROP COLUMN attempt_at,
    DROP COLUMN attempt;
