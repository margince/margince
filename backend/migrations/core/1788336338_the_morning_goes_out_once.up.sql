-- The morning brief, mailed to the rep whose day it is — at most once.
--
-- The same claim the weekly takes (1787822400), for the same reason and with
-- the same name. There is no delivery ledger and no receipt: the transport is a
-- synchronous SMTP call with no retry identity, so the only way to bound
-- duplicates is to claim the attempt BEFORE dialling the relay. The claim is a
-- conditional UPDATE exactly one transaction can win, and every pass after it
-- finds the row claimed and does nothing.
--
-- WHY THE STAKES ARE HIGHER HERE THAN ON THE WEEKLY. A brief run is per rep per
-- LOCAL DAY, so this lane runs five times a week where the weekly runs once. A
-- duplicate is not one embarrassing Monday; it is a person learning that the
-- product mails them twice every morning, and the fastest way to teach someone
-- to filter a message is to send it to them twice.
--
-- It is not called sent_at. What the installation observes is that an attempt
-- was made — the trade loses the mail on any post-claim failure, a crash before
-- the dial, a refused envelope, a connection dropped mid-body — and a column
-- called sent_at would claim a delivery nobody watched land.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run ADD COLUMN mail_attempted_at timestamptz;

-- Why the attempt failed, when it did. NULL beside a stamp means the relay
-- accepted the message, so a missing brief is diagnosable from the row rather
-- than only from a log line nobody kept.
ALTER TABLE brief_run ADD COLUMN mail_error text;

ALTER TABLE brief_run
    ADD CONSTRAINT brief_run_mail_error_needs_an_attempt
    CHECK (mail_error IS NULL OR mail_attempted_at IS NOT NULL);

-- Bounded for the same reason the weekly's is: a driver error's full text is
-- unbounded, and a row nothing can render helps nobody.
ALTER TABLE brief_run
    ADD CONSTRAINT brief_run_mail_error_length
    CHECK (mail_error IS NULL OR char_length(mail_error) <= 500);
