-- What each captured email says it replies to: its In-Reply-To and every id in
-- its References header, one row per referenced Message-ID.
--
-- thread_key roots a message on the FIRST References entry, and mail programs
-- shorten that header: iPhone Mail sends only the direct parent, so one
-- conversation arrives under a new root at every reply it makes. The links
-- kept here are what lets two threads that turn out to be one conversation be
-- joined later, in whichever order their messages arrive. A backfill walks a
-- mailbox newest first, so the reply is routinely stored before the message it
-- answers.
--
-- The referenced id is the sender's text, not a proven fact. It is only ever
-- acted on when the same seat holds both messages (capture/threadjoin.go).
SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_mail_reference (
    activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- The referenced Message-ID with its angle brackets stripped, the spelling
    -- activity_identity and activity.source_id use for mail.
    referenced_id text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    PRIMARY KEY (activity_id, referenced_id)
);

-- The reverse read: which stored messages reply to this one.
CREATE INDEX activity_mail_reference_referenced_idx ON activity_mail_reference (referenced_id);
