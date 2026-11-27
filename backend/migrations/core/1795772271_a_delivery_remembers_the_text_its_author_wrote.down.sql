SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    DROP CONSTRAINT IF EXISTS comms_outbound_resent_from_fkey,
    DROP COLUMN IF EXISTS resent_from,
    DROP COLUMN IF EXISTS send_claims,
    DROP COLUMN IF EXISTS authored_html_len,
    DROP COLUMN IF EXISTS authored_body_len;
