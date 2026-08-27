-- The weekly retrospective, mailed to the rep who owns it — at most once.
--
-- THE COLUMN IS NOT CALLED sent_at, and the name is the design.
--
-- There is no delivery ledger here and no receipt. The transport is a
-- synchronous SMTP call with no retry identity, so the only way to bound
-- duplicates is to claim the attempt BEFORE calling the relay: the claim is a
-- conditional UPDATE that exactly one transaction can win, and every retry
-- after it finds the row already claimed and does nothing.
--
-- That trade loses the mail on ANY post-claim failure — a crash before the
-- relay is even dialled, a refused envelope, a connection dropped mid-body.
-- So what this column records is that an attempt was made, which is the only
-- thing this installation actually observes. A column called sent_at would
-- claim a delivery nobody watched land.
--
-- The alternative is worse for this product: a weekly retrospective mailed
-- twice is a person being told their week twice, on the one morning the mail
-- exists to make calm.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review ADD COLUMN mail_attempted_at timestamptz;

-- Why the attempt failed, when it did. NULL beside a stamp means the relay
-- accepted the message; a missing weekly is then diagnosable from the row
-- rather than only from a log line nobody kept.
ALTER TABLE weekly_review ADD COLUMN mail_error text;

ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_mail_error_needs_an_attempt
    CHECK (mail_error IS NULL OR mail_attempted_at IS NOT NULL);

-- Bounded for the same reason the narrative is: a driver error's full text is
-- unbounded, and a row nothing can render helps nobody.
ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_mail_error_length
    CHECK (mail_error IS NULL OR char_length(mail_error) <= 500);
