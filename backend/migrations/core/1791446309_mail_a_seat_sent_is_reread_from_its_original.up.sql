-- Mail a seat sent from another address of theirs, captured before capture
-- read it as outbound, still reads as received from the seat's own address.
-- A background pass re-reads each such email's stored original and claims the
-- ones the seat wrote. The verdict table records each judgement, so a restart
-- resumes rather than re-reading from the top. The cutoff row bounds the pass
-- to mail captured before this migration: mail captured since is live
-- capture's to read, on the provider's own sent filing.
--
-- No workspace column and no policy, matching activity_meeting_rsvp_backfill:
-- the pass runs inside the workspace transaction every capture write uses.

SET LOCAL lock_timeout = '3s';

CREATE TABLE activity_own_sent_mail_repair (
    activity_id uuid NOT NULL,
    settled_at timestamptz DEFAULT now() NOT NULL,
    --   `claimed`        the claim turned the row round as the seat's outbound mail.
    --   `unchanged`      the seat may claim it, but the row no longer read as
    --                    received from the seat, or was archived meanwhile.
    --   `delivered`      the original records a delivery hop; it stays received.
    --   `not_the_seats`  its sender was not an address the seat alone had proven
    --                    when judged; offered again while it is one.
    --   `no_recipient`   it names nobody but the seat as a recipient.
    --   `unreadable`     the original does not parse as a message.
    --   `erased`         its recipient's address was erased; it stays received.
    outcome text NOT NULL,
    CONSTRAINT activity_own_sent_mail_repair_pkey PRIMARY KEY (activity_id),
    CONSTRAINT activity_own_sent_mail_repair_activity_fk
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_own_sent_mail_repair_outcome_check
        CHECK (outcome IN ('claimed', 'unchanged', 'delivered', 'not_the_seats', 'no_recipient', 'unreadable', 'erased'))
);

CREATE TABLE activity_own_sent_mail_repair_cutoff (
    singleton boolean DEFAULT true NOT NULL,
    captured_before timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT activity_own_sent_mail_repair_cutoff_pkey PRIMARY KEY (singleton),
    CONSTRAINT activity_own_sent_mail_repair_cutoff_singleton_check CHECK (singleton)
);

INSERT INTO activity_own_sent_mail_repair_cutoff DEFAULT VALUES;
