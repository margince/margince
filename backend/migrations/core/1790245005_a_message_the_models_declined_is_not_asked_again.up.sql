SET LOCAL lock_timeout = '5s';

-- When every rung of a sweep's ladder declines one message — a provider's safety
-- filter withholding the answer, say — that is the message's outcome, and the
-- sweep must not send it again on every tick. Each question gets its own stamp,
-- because a message one prompt declines another may answer. Nullable with no
-- default, so adding them rewrites nothing.
ALTER TABLE activity
    ADD COLUMN capture_label_declined_at timestamptz,
    ADD COLUMN owed_verdict_declined_at timestamptz;
