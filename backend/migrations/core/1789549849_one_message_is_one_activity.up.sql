-- One message is one activity, whichever provider hands it over.
--
-- An activity's own (source_system, source_id) is where the FIRST arrival filed
-- it: 'email' + Message-ID for captured mail, an importer's own namespace for a
-- record it carried across. Those keys never meet, so the same email arriving
-- from a mailbox and from an import became two rows.
--
-- This table is the identity the two agree on. The primary key is the whole
-- point: one external identity resolves to exactly ONE activity, and a second
-- arrival claiming it collides here rather than quietly creating a second row.
-- Both ingestion paths claim through this table before they insert, so the
-- collision is what makes the claim atomic.

CREATE TABLE activity_identity (
    -- 'mail' for an RFC 5322 Message-ID, 'meeting' for a calendar occurrence.
    identity_kind text NOT NULL,
    -- The Message-ID with its angle brackets stripped, or the iCal UID and the
    -- occurrence within it. A recurring series shares one UID across every
    -- occurrence, so a UID alone would collapse a weekly call into one row.
    identity_key text NOT NULL,
    activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- Who asserted this identity: 'connector:gmail' for a mailbox that held the
    -- message, a principal stamp for a client that stated it. A Message-ID is
    -- typed by whoever sent the message, so a claim from a client is an
    -- assertion and a claim from a connector is an observation — and the
    -- resolver has to be able to tell them apart before it binds one to the
    -- other.
    attested_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT activity_identity_kind_check CHECK (identity_kind IN ('mail', 'meeting')),
    PRIMARY KEY (identity_kind, identity_key)
);

-- Every read from the activity's side: what else is this message known as, and
-- what has to move when two rows merge.
CREATE INDEX activity_identity_activity_idx ON activity_identity (activity_id);
