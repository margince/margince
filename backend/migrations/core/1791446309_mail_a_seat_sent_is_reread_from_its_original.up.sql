-- Mail a seat sent from another address of theirs, captured before capture
-- read it as outbound, still reads as received from the seat's own address.
-- A background pass re-reads each such email's stored original and claims the
-- ones the seat wrote. This table records each verdict, so a restart resumes
-- rather than re-reading from the top and an email judged once is not offered
-- again.
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
    --   `not_the_seats`  its sender is not an address the seat held when judged,
    --                    or it names nobody but the seat as a recipient. Judged
    --                    again once the seat holds a new address or mailbox.
    --   `unreadable`     the original does not parse as a message.
    --   `erased`         its recipient's address was erased; it stays received.
    outcome text NOT NULL,
    CONSTRAINT activity_own_sent_mail_repair_pkey PRIMARY KEY (activity_id),
    CONSTRAINT activity_own_sent_mail_repair_activity_fk
        FOREIGN KEY (activity_id) REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_own_sent_mail_repair_outcome_check
        CHECK (outcome IN ('claimed', 'unchanged', 'delivered', 'not_the_seats', 'unreadable', 'erased'))
);
