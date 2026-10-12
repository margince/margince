-- Which captured items the tag suggestion model has read, and against which
-- suggestible tags. An item is read again only when that vocabulary changes,
-- so each mail, meeting, note or call is read once per vocabulary.
SET LOCAL lock_timeout = '5s';

CREATE TABLE tag_interest_read (
    activity_id uuid NOT NULL,
    -- tagsuggest.VocabularyDigest: the suggestible tags and the prompt's rules.
    vocabulary_digest text NOT NULL,
    read_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT tag_interest_read_pkey PRIMARY KEY (activity_id)
);

ALTER TABLE tag_interest_read
    ADD CONSTRAINT tag_interest_read_activity_id_fkey FOREIGN KEY (activity_id)
    REFERENCES activity(id) ON DELETE CASCADE;
