-- A typed link between two core records: a follow-up deal, a renewal, a related
-- company. It joins any two of deal, company and contact. The type is one of
-- core's words or `ext:<unit>:<type>`; which unit types exist is the composed
-- set's to say, so the database holds only the shape and the API holds the set.
--
-- The ends carry no foreign key because each is one of three tables. The module
-- reads an end through its own table and hides a link whose end is archived, so
-- archiving keeps the link and unarchiving brings it back. Erasing a contact
-- deletes its links in the erasure transaction.
--
-- A link has one direction (from follows up to), and the same pair may hold
-- several types. The reverse of a pair is a different link, not a duplicate.
SET LOCAL lock_timeout = '5s';

CREATE TABLE record_link (
    id uuid DEFAULT uuidv7() NOT NULL,
    from_type text NOT NULL,
    from_id uuid NOT NULL,
    to_type text NOT NULL,
    to_id uuid NOT NULL,
    link_type text NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT record_link_pkey PRIMARY KEY (id),
    CONSTRAINT record_link_from_type_check CHECK (from_type IN ('deal', 'company', 'contact')),
    CONSTRAINT record_link_to_type_check CHECK (to_type IN ('deal', 'company', 'contact')),
    CONSTRAINT record_link_type_check CHECK (
        link_type IN ('follow_up_of', 'renews', 'related_to')
        OR link_type ~ '^ext:[a-z0-9]+(-[a-z0-9]+)*:[a-z][a-z0-9_]{0,31}$'),
    CONSTRAINT record_link_not_itself CHECK (from_type <> to_type OR from_id <> to_id)
);

CREATE UNIQUE INDEX uq_record_link_pair
    ON record_link (from_type, from_id, to_type, to_id, link_type);
CREATE INDEX idx_record_link_to ON record_link (to_type, to_id);

GRANT SELECT, INSERT, DELETE ON TABLE record_link TO margince_app;
