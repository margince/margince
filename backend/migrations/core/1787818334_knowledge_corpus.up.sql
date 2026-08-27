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
    -- Which shipped body of prose this corpus IS, when it is not a user's.
    -- NULL is a corpus somebody in the workspace defined; 'handbook' is the
    -- operator handbook the binary carries and reconciles on start.
    --
    -- A column rather than a match on `name`: the seeding has to find the
    -- corpus it owns on every boot, and a name is the one thing an
    -- administrator is free to change. Matching on it would let a rename either
    -- orphan the handbook or, worse, walk the seeding into overwriting a
    -- corpus somebody built by hand.
    managed_source text,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT knowledge_corpus_min_similarity_range CHECK (min_similarity >= 0 AND min_similarity <= 1),
    -- The set is closed here rather than left to the writer. A typo'd source
    -- reconciles against nothing and would sit in the table looking managed
    -- while the boot step creates a second corpus beside it every release.
    CONSTRAINT knowledge_corpus_managed_source_known
        CHECK (managed_source IS NULL OR managed_source IN ('handbook'))
);

ALTER TABLE knowledge_corpus ADD CONSTRAINT knowledge_corpus_pkey PRIMARY KEY (id);

-- At most one default, held by the schema rather than by application code:
-- two defaults is a palette that asks a different corpus depending on row
-- order.
CREATE UNIQUE INDEX knowledge_corpus_one_default
    ON knowledge_corpus ((default_ask)) WHERE default_ask AND archived_at IS NULL;

-- One live corpus per managed source. The boot step reconciles into the corpus
-- it finds, so two of them would mean each boot picks by row order and half the
-- handbook lands in one and half in the other.
CREATE UNIQUE INDEX knowledge_corpus_one_per_managed_source
    ON knowledge_corpus (managed_source) WHERE managed_source IS NOT NULL AND archived_at IS NULL;

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
    -- Set on a document the binary ships, matching its corpus's own
    -- managed_source. It is what tells the boot reconciliation which rows are
    -- ITS to update and delete, so a document an administrator uploaded into
    -- the handbook corpus is left alone rather than swept away as "not a page
    -- this release carries".
    managed_source text,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT knowledge_document_status_check
        CHECK (ingest_status IN ('queued', 'running', 'done', 'failed')),
    -- A failure that does not say why is a support ticket. A success that
    -- explains itself is noise.
    CONSTRAINT knowledge_document_detail_shape
        CHECK ((ingest_status = 'failed') = (ingest_detail IS NOT NULL)),
    CONSTRAINT knowledge_document_managed_source_known
        CHECK (managed_source IS NULL OR managed_source IN ('handbook'))
);

ALTER TABLE knowledge_document ADD CONSTRAINT knowledge_document_pkey PRIMARY KEY (id);

-- One live document per distinct content per corpus, held by the SCHEMA rather
-- than by the upload being careful.
--
-- The upload reads before it inserts, and two uploads of identical bytes can
-- both run that read before either commits — so the check that produces the
-- useful refusal cannot also be the thing that guarantees it. The same posture
-- as knowledge_chunk_one_per_span below: the application says WHY, the index
-- says never.
--
-- Partial on archived_at, because an archived document is out of the corpus and
-- must not stop the same file being filed again.
CREATE UNIQUE INDEX knowledge_document_one_per_content
    ON knowledge_document (corpus_id, checksum) WHERE archived_at IS NULL;

-- One live document per page name inside a managed corpus. The boot
-- reconciliation looks its rows up by (corpus, filename) — the page name is
-- what the release ships and what a citation shows — so two rows for one page
-- would leave it updating whichever came back first and citing the other.
-- Scoped to managed rows: a person uploading two files of the same name into
-- their own corpus is their business, and the checksum index above already
-- refuses the case that actually duplicates content.
CREATE UNIQUE INDEX knowledge_document_one_per_managed_page
    ON knowledge_document (corpus_id, filename)
    WHERE managed_source IS NOT NULL AND archived_at IS NULL;

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
    -- Where this span STARTS in the document it was cut from, 1-based, so a
    -- citation can say which line to open the file at.
    --
    -- Stored rather than derived, because deriving it at ask time would mean
    -- re-reading the document's bytes for every answer — the ingest already
    -- holds the whole text and counting there costs one pass.
    --
    -- Nullable for the rows written before this column existed: a passage with
    -- no start line yields a claim with no line, which is the honest answer.
    -- A line number that points at the wrong line is worse than none.
    -- The 1-based line a passage begins on, NULL when the ingest that wrote it
    -- did not record one. Constrained positive because the read path treats
    -- only ZERO as "unknown" (ask.go coalesces NULL to 0), so a negative would
    -- pass straight through as a real line and reach a reader as a citation
    -- pointing at a place no file has. Nothing writes one today — the chunker
    -- counts newlines and adds one — which is what makes this cheap to hold
    -- here rather than a rule the next writer has to remember.
    start_line integer,
    embed_identity text,
    embedding vector,
    embedded_at timestamptz,
    archived_at timestamptz,
    -- The vector and the identity that produced it are written together or not
    -- at all: a row carrying an identity and no vector is retrievable and
    -- unrankable, which is worse than an unembedded row.
    CONSTRAINT knowledge_chunk_embed_pairing
        CHECK ((embed_identity IS NULL) = (embedding IS NULL)),
    CONSTRAINT knowledge_chunk_start_line_positive
        CHECK (start_line IS NULL OR start_line > 0)
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
