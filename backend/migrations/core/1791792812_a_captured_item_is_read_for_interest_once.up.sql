-- What the tag suggestion model answered for each captured item, and against
-- which tags and which text. An item is read again only when the suggestible
-- tags or its own text change; the answer is replayed into suggestions on every
-- pass, so a pair refused today is raised when it becomes writable.
SET LOCAL lock_timeout = '5s';

CREATE TABLE tag_interest_read (
    activity_id uuid NOT NULL,
    -- tagsuggest.VocabularyDigest: the suggestible tags and the prompt's rules.
    vocabulary_digest text NOT NULL,
    -- A hash of the subject and body text the prompt read.
    content_digest text NOT NULL,
    -- The tags the model found the item shows interest in; empty for none.
    tag_ids uuid[] DEFAULT '{}' NOT NULL,
    read_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT tag_interest_read_pkey PRIMARY KEY (activity_id)
);

ALTER TABLE tag_interest_read
    ADD CONSTRAINT tag_interest_read_activity_id_fkey FOREIGN KEY (activity_id)
    REFERENCES activity(id) ON DELETE CASCADE;
