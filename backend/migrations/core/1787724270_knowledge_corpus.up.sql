-- A corpus of user-uploaded documents, its documents, and the chunks the ask
-- retrieves over.
SET LOCAL lock_timeout = '5s';

CREATE TABLE knowledge_corpus (
    id uuid DEFAULT uuidv7() NOT NULL,
    name text NOT NULL,
    description text,
    topic_statement text NOT NULL,
    min_similarity double precision DEFAULT 0.35 NOT NULL,
    default_ask boolean DEFAULT false NOT NULL,
    reindexing boolean DEFAULT false NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT knowledge_corpus_min_similarity_range CHECK (min_similarity >= 0 AND min_similarity <= 1)
);

ALTER TABLE knowledge_corpus ADD CONSTRAINT knowledge_corpus_pkey PRIMARY KEY (id);

-- At most one default, held by the schema rather than by application code:
-- two defaults is a palette that asks a different corpus depending on row
-- order.
CREATE UNIQUE INDEX knowledge_corpus_one_default
    ON knowledge_corpus ((default_ask)) WHERE default_ask AND archived_at IS NULL;

CREATE TABLE knowledge_document (
    id uuid DEFAULT uuidv7() NOT NULL,
    corpus_id uuid NOT NULL,
    filename text NOT NULL,
    content_type text NOT NULL,
    byte_size bigint NOT NULL,
    storage_key text NOT NULL,
    checksum text NOT NULL,
    ingest_status text DEFAULT 'queued' NOT NULL,
    -- When the CURRENT attempt began, which updated_at cannot say: the trigger
    -- moves that for any write, so a row touched for any other reason would
    -- look like a fresh attempt. The sweep that closes an ingest whose worker
    -- went away needs the attempt's own start and nothing else's.
    ingest_started_at timestamptz,
    ingest_detail text,
    chunk_count integer DEFAULT 0 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT knowledge_document_status_check
        CHECK (ingest_status IN ('queued', 'running', 'done', 'failed')),
    -- A failure that does not say why is a support ticket. A success that
    -- explains itself is noise.
    CONSTRAINT knowledge_document_detail_shape
        CHECK ((ingest_status = 'failed') = (ingest_detail IS NOT NULL))
);

ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_pkey PRIMARY KEY (id);

-- A document's audit image names its filename and audit_log is append-only, so
-- a document may only die by a path that read it first: deleting a corpus that
-- still has documents is refused rather than silently taking them with it.
ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_corpus_fk
    FOREIGN KEY (corpus_id) REFERENCES knowledge_corpus (id) ON DELETE RESTRICT;
CREATE INDEX knowledge_document_by_corpus ON knowledge_document (corpus_id, archived_at);

CREATE TABLE knowledge_chunk (
    id uuid DEFAULT uuidv7() NOT NULL,
    corpus_id uuid NOT NULL,
    document_id uuid NOT NULL,
    chunk_ix integer NOT NULL,
    text text NOT NULL,
    chunk_hash text NOT NULL,
    embed_identity text,
    embedding vector,
    embedded_at timestamptz,
    archived_at timestamptz,
    -- The vector and the identity that produced it are written together or not
    -- at all: a row carrying an identity and no vector is retrievable and
    -- unrankable, which is worse than an unembedded row.
    CONSTRAINT knowledge_chunk_embed_pairing
        CHECK ((embed_identity IS NULL) = (embedding IS NULL))
);

ALTER TABLE knowledge_chunk ADD CONSTRAINT knowledge_chunk_pkey PRIMARY KEY (id);

-- Chunks carry no audit image, so a cascade orphans nothing: they are derived
-- text that only the document they were cut from gives meaning.
ALTER TABLE knowledge_chunk ADD CONSTRAINT knowledge_chunk_document_fk
    FOREIGN KEY (document_id) REFERENCES knowledge_document (id) ON DELETE CASCADE;

-- One row per span per document, held by the SCHEMA rather than by the ingest
-- being careful.
--
-- Two attempts at one document can overlap: River dedupes a queued ingest by
-- args, but a worker declared dead while it is still running has its row
-- rescued and re-run, and both attempts then delete the previous chunks and
-- insert their own. Nothing in the application would notice — the rows differ
-- only by id — and the corpus would hold every passage twice, cite duplicates,
-- report a chunk_count half of what it holds, and pass the per-corpus ceiling
-- while over it. Here the second insert is refused, the attempt fails, and
-- River retries it cleanly.
CREATE UNIQUE INDEX knowledge_chunk_one_per_span
    ON knowledge_chunk (document_id, chunk_ix);

-- The ask's hot path and the readiness count both key on this pair. There is
-- deliberately no vector index: the column is unbounded width, so an index over
-- it is unusable for the same reason search carries none.
CREATE INDEX knowledge_chunk_retrieval
    ON knowledge_chunk (corpus_id, embed_identity) WHERE archived_at IS NULL;

-- updated_at is the trigger's, not the caller's: neither table carries a
-- version column, so set_updated_at rather than its version-bumping sibling.
-- knowledge_chunk gets none — a chunk is rewritten wholesale by the next ingest
-- attempt and has no updated_at to keep honest.
CREATE TRIGGER knowledge_corpus_touch BEFORE UPDATE ON knowledge_corpus
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
CREATE TRIGGER knowledge_document_touch BEFORE UPDATE ON knowledge_document
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE knowledge_corpus TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE knowledge_document TO margince_app;
GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE knowledge_chunk TO margince_app;
