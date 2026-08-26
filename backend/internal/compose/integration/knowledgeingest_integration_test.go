// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The ingest suite: what a document becomes, and what a second attempt at the
// same document must NOT become.
//
// Every case here runs the real store against the real database and the real
// chunker. Nothing is stubbed but the object store, which is the shared
// in-memory implementation rather than a double of this module's own writing —
// a test that supplies its own version of production proves nothing about
// production.

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/platform/blobstore"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

// ingestEnv is one corpus, its store, and an admin context over both.
type ingestEnv struct {
	env    *Env
	store  *knowledge.Store
	ctx    context.Context
	corpus ids.UUID
	// blob is the object store the uploads wrote to, kept so a delete can be
	// checked against the bytes rather than only against the rows.
	blob blobstore.Store
	// queued records the document ids the upload asked to have ingested, so a
	// test can assert the enqueue happened inside the write rather than mock it.
	queued []ids.UUID
}

func newIngestEnv(t *testing.T) *ingestEnv {
	t.Helper()
	e := Setup(t)
	ie := &ingestEnv{env: e}
	ie.blob = blobstore.NewMemory()
	ie.store = knowledge.NewStore(e.DB()).WithBlobstore(ie.blob)
	ie.ctx = e.As(e.Rep1, nil, corpusAdminPerms)
	made, err := ie.store.CreateCorpus(ie.ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create corpus: %v", err)
	}
	ie.corpus = ids.UUID(made.Id)
	return ie
}

// queue is the QueueIngest seam. It records rather than enqueues: River is not
// what these tests are about, and a real runner would put a clock between the
// upload and the assertion.
func (ie *ingestEnv) queue(_ context.Context, _ pgx.Tx, documentID ids.UUID) error {
	ie.queued = append(ie.queued, documentID)
	return nil
}

func (ie *ingestEnv) upload(t *testing.T, name, contentType, body string) ids.UUID {
	t.Helper()
	doc, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    name,
		ContentType: contentType,
		Content:     strings.NewReader(body),
	}, ie.queue)
	if err != nil {
		t.Fatalf("upload %s: %v", name, err)
	}
	return ids.UUID(doc.Id)
}

// ingest runs one full attempt the way the worker does.
func (ie *ingestEnv) ingest(t *testing.T, documentID ids.UUID) int {
	t.Helper()
	chunks, err := ie.attempt(t, documentID)
	if err != nil {
		t.Fatalf("ingest: %v", err)
	}
	return chunks
}

func (ie *ingestEnv) attempt(t *testing.T, documentID ids.UUID) (int, error) {
	t.Helper()
	src, err := ie.store.BeginIngest(ie.ctx, documentID)
	if err != nil {
		return 0, err
	}
	body, err := ie.store.OpenDocument(ie.ctx, src.StorageKey)
	if err != nil {
		return 0, err
	}
	defer func() {
		if cerr := body.Close(); cerr != nil {
			t.Errorf("closing the stored document: %v", cerr)
		}
	}()
	raw, err := io.ReadAll(body)
	if err != nil {
		return 0, err
	}
	chunks := knowledge.ChunkText(string(raw))
	if err := ie.store.WriteChunks(ie.ctx, documentID, src.CorpusID, chunks); err != nil {
		return 0, err
	}
	return len(chunks), ie.store.FinishIngest(ie.ctx, documentID, len(chunks))
}

// prose is a body long enough to cut into several passages, so a doubling is
// visible as a count rather than as a single duplicate row.
func prose(paragraphs int) string {
	var b strings.Builder
	for i := 0; i < paragraphs; i++ {
		b.WriteString(strings.Repeat("The corpus answers only from what a workspace filed in it. ", 12))
		b.WriteString("\n\n")
	}
	return b.String()
}

// The upload queues its ingest INSIDE the transaction that writes the row: a
// job enqueued outside it can wake before the row exists, or survive a rollback
// that leaves it pointing at nothing.
func TestAnUploadedDocumentLandsQueuedWithItsIngestAsked(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(2))

	if len(ie.queued) != 1 || ie.queued[0] != docID {
		t.Fatalf("the upload queued %v, want exactly the document it wrote", ie.queued)
	}
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 1 {
		t.Fatalf("listed %d documents, want 1", len(docs))
	}
	if docs[0].IngestStatus != "queued" {
		t.Fatalf("ingest_status = %q, want queued", docs[0].IngestStatus)
	}
}

// A retry must not double the corpus. The attempt begins by deleting whatever
// the previous attempt wrote, so a crash halfway through is invisible to the
// next one.
func TestARetriedIngestDoesNotDoubleTheChunks(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(3))

	first := ie.ingest(t, docID)
	if first < 2 {
		t.Fatalf("the fixture produced %d passages; it must produce several for a doubling to be visible", first)
	}
	second := ie.ingest(t, docID)
	if second != first {
		t.Fatalf("the second attempt cut %d passages, the first cut %d", second, first)
	}
	if live := liveChunkCount(t, ie.env, ie.corpus); live != first {
		t.Fatalf("the corpus holds %d passages after two attempts at one document, want %d", live, first)
	}
}

// The same rule for an attempt that DIED mid-flight: BeginIngest is what clears
// the debris, so a half-written attempt costs the next one nothing.
func TestAnAttemptThatDiedMidFlightLeavesTheNextOneClean(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(3))

	// A first attempt that wrote its chunks and then died before finishing:
	// the row stays `running` and the chunks are on the floor.
	src, err := ie.store.BeginIngest(ie.ctx, docID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	half := knowledge.ChunkText(prose(3))
	if err := ie.store.WriteChunks(ie.ctx, docID, src.CorpusID, half); err != nil {
		t.Fatalf("write chunks: %v", err)
	}

	full := ie.ingest(t, docID)
	if live := liveChunkCount(t, ie.env, ie.corpus); live != full {
		t.Fatalf("the corpus holds %d passages, want the %d the finished attempt wrote", live, full)
	}
}

// A terminally failed document leaves no chunks behind: a chunk it left would
// be retrievable and citable out of a document the screen shows as failed.
func TestATerminallyFailedIngestLeavesNoChunks(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(3))
	ie.ingest(t, docID)
	if liveChunkCount(t, ie.env, ie.corpus) == 0 {
		t.Fatal("the fixture wrote no passages, so the failure below would prove nothing")
	}

	if err := ie.store.FailIngest(ie.ctx, docID, "the stored file could not be read"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if live := liveChunkCount(t, ie.env, ie.corpus); live != 0 {
		t.Fatalf("%d passage(s) survived a terminally failed ingest", live)
	}
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if docs[0].IngestStatus != "failed" {
		t.Fatalf("ingest_status = %q, want failed", docs[0].IngestStatus)
	}
	// The reason is the whole point of listing a failed document: a corpus
	// quietly short of a file nobody can name answers worse than an empty one.
	if docs[0].IngestDetail == nil || *docs[0].IngestDetail == "" {
		t.Fatal("a failed document carries no reason")
	}
}

// The refusal names what IS accepted. A PDF arriving as an empty corpus is
// worse than a PDF refused.
func TestAPDFIsRefusedNamingTheAcceptedTypes(t *testing.T) {
	ie := newIngestEnv(t)
	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "handbook.pdf",
		ContentType: "application/pdf",
		Content:     strings.NewReader("%PDF-1.7"),
	}, ie.queue)

	var unsupported *knowledge.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("upload of a PDF = %v, want UnsupportedTypeError", err)
	}
	for _, accepted := range []string{"text/plain", "text/markdown", "text/csv", "application/json"} {
		if !strings.Contains(unsupported.Error(), accepted) {
			t.Fatalf("the refusal does not name %s: %q", accepted, unsupported.Error())
		}
	}
	// Refused before anything was written: a rejected file leaves no row for a
	// screen to list and no object for a sweep to find.
	if len(ie.queued) != 0 {
		t.Fatalf("a refused upload queued %v", ie.queued)
	}
	if n := liveDocumentCount(t, ie.env, ie.corpus); n != 0 {
		t.Fatalf("a refused upload wrote %d document row(s)", n)
	}
}

// A browser sends `text/markdown; charset=utf-8`. Matching the parameters as
// part of the type would refuse every real upload while passing every test that
// spelled the type bare.
func TestAContentTypeCarryingParametersIsStillAccepted(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown; charset=utf-8", prose(1))
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if docs[0].Id.String() != docID.String() {
		t.Fatalf("listed %s, want the uploaded %s", docs[0].Id, docID)
	}
	// Stored as the bare media type: the column is read back by the ingest and
	// shown on the screen, and two spellings of one type is two answers.
	if docs[0].ContentType != "text/markdown" {
		t.Fatalf("content_type = %q, want the bare media type", docs[0].ContentType)
	}
}

// Archive wins the race by being re-read inside the chunk-write transaction.
// Without that re-read an attempt already past the archive would write live
// chunks under an archived document a moment later.
func TestADocumentArchivedMidIngestLeavesNoLiveChunk(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(2))

	src, err := ie.store.BeginIngest(ie.ctx, docID)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	// The corpus archives while the attempt is between reading and writing —
	// which is the only window in which this can happen at all.
	if err := ie.store.ArchiveCorpus(ie.ctx, ie.corpus); err != nil {
		t.Fatalf("archive: %v", err)
	}
	err = ie.store.WriteChunks(ie.ctx, docID, src.CorpusID, knowledge.ChunkText(prose(2)))
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("writing chunks under an archived document = %v, want ErrNotFound", err)
	}
	if live := liveChunkCount(t, ie.env, ie.corpus); live != 0 {
		t.Fatalf("%d live passage(s) under an archived corpus", live)
	}
}

// An empty file is not a failure: it is a document with nothing in it, and
// saying so is more useful than a refusal the uploader cannot act on.
func TestAnEmptyDocumentIngestsToNoPassagesAndSucceeds(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "empty.md", "text/markdown", "   \n\n  ")

	if n := ie.ingest(t, docID); n != 0 {
		t.Fatalf("an empty document cut %d passages", n)
	}
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if docs[0].IngestStatus != "done" {
		t.Fatalf("ingest_status = %q, want done", docs[0].IngestStatus)
	}
}

// The upload with no object store bound refuses as OUR fault. Carrying on would
// write a row promising bytes that are nowhere.
func TestAnUploadWithNoObjectStoreRefusesRatherThanWritingARow(t *testing.T) {
	ie := newIngestEnv(t)
	storeless := knowledge.NewStore(ie.env.DB())

	_, err := storeless.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "operating.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(prose(1)),
	}, ie.queue)
	if !errors.Is(err, knowledge.ErrBlobstoreUnconfigured) {
		t.Fatalf("upload with no object store = %v, want ErrBlobstoreUnconfigured", err)
	}
	if n := liveDocumentCount(t, ie.env, ie.corpus); n != 0 {
		t.Fatalf("%d document row(s) written with nowhere to put the bytes", n)
	}
}

// Uploading into a corpus that does not exist is absent, not a stored orphan.
func TestAnUploadIntoAMissingCorpusStoresNothing(t *testing.T) {
	ie := newIngestEnv(t)
	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ids.NewV7(),
		Filename:    "operating.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(prose(1)),
	}, ie.queue)
	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("upload into an unknown corpus = %v, want ErrNotFound", err)
	}
	if len(ie.queued) != 0 {
		t.Fatalf("a refused upload queued %v", ie.queued)
	}
}

// A rep may read a corpus's documents and may not file one: the document grant
// is admin/ops-only because uploading puts third-party prose into the body
// every seat then asks.
func TestARepMayListDocumentsAndMayNotUploadOne(t *testing.T) {
	ie := newIngestEnv(t)
	ie.upload(t, "operating.md", "text/markdown", prose(1))
	rep := ie.env.As(ie.env.Rep1, nil, corpusRepPerms)

	if _, err := ie.store.ListDocuments(rep, ie.corpus); err != nil {
		t.Fatalf("a rep must be able to list a corpus's documents: %v", err)
	}
	_, err := ie.store.UploadDocument(rep, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "reps-own.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(prose(1)),
	}, ie.queue)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep upload = %v, want ErrPermissionDenied", err)
	}
}

// Listing an unknown corpus is absent rather than an empty list: "no documents"
// and "no such corpus" are different answers and the screen renders them
// differently.
func TestListingDocumentsOfAMissingCorpusIsNotFound(t *testing.T) {
	ie := newIngestEnv(t)
	if _, err := ie.store.ListDocuments(ie.ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("list of an unknown corpus = %v, want ErrNotFound", err)
	}
}

// The ceiling is the CORPUS's, not the file's — N small uploads exceed any
// per-file cap, and what the ceiling protects is the ask, which scans every
// passage in the corpus with no vector index to lean on.
//
// The corpus is filled by one statement rather than by 20,000 uploads: what is
// under test is the refusal, and a test that took a minute to reach it would be
// deleted by the next person who ran the suite.
func TestADocumentCrossingTheCorpusPassageCeilingIsRefused(t *testing.T) {
	ie := newIngestEnv(t)
	filler := ie.upload(t, "filler.md", "text/markdown", prose(1))
	fillToPassageCeiling(t, ie.env, ie.corpus, filler)

	// Distinct bytes from the filler, or the duplicate refusal fires first and
	// this case would never reach the ceiling it names.
	docID := ie.upload(t, "one-more.md", "text/markdown", prose(2))
	_, err := ie.attempt(t, docID)

	var full *knowledge.CorpusFullError
	if !errors.As(err, &full) {
		t.Fatalf("ingest past the ceiling = %v, want CorpusFullError", err)
	}
	// The refusal states the number. "Too large" without one leaves the
	// uploader guessing at which file to remove.
	if !strings.Contains(full.Error(), "20000") {
		t.Fatalf("the refusal does not name the ceiling: %q", full.Error())
	}
	// Nothing of the refused document landed: a partial write would be counted
	// by the next attempt's own ceiling check.
	if live := liveChunkCount(t, ie.env, ie.corpus); live != knowledgePassageCeiling {
		t.Fatalf("the corpus holds %d passages after a refused ingest, want %d", live, knowledgePassageCeiling)
	}
}

// knowledgePassageCeiling mirrors the module's unexported maxCorpusChunks,
// which this package cannot import.
//
// Held by: TestADocumentCrossingTheCorpusPassageCeilingIsRefused (this file) —
// it fills the corpus to exactly this number and then requires a refusal, so a
// ceiling moved in the module without moving this number fails here rather than
// leaving a stale constant that reads as agreement.
const knowledgePassageCeiling = 20000

func fillToPassageCeiling(t *testing.T, e *Env, corpusID, documentID ids.UUID) {
	t.Helper()
	e.WsExec(t,
		`INSERT INTO knowledge_chunk (corpus_id, document_id, chunk_ix, text, chunk_hash)
		 SELECT $1, $2, g, 'filler ' || g, md5(g::text)
		 FROM generate_series(1, $3) AS g`,
		corpusID, documentID, knowledgePassageCeiling)
}

// A terminally failed ingest takes its stored file with it.
//
// No further attempt will read the bytes once the ingest gives up, so leaving
// them is an unbounded store of files nothing will open. The corpus ceiling
// bounds PASSAGES, not storage: a document refused by that ceiling would
// otherwise keep its megabytes forever, once per attempt anyone cares to make.
func TestATerminallyFailedIngestTakesItsStoredFileWithIt(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(2))
	key := wsText(t, ie.env, `SELECT storage_key FROM knowledge_document WHERE id = $1`, docID)
	if _, _, err := ie.blob.Get(ie.ctx, key); err != nil {
		t.Fatalf("the upload stored nothing, so this proves nothing: %v", err)
	}

	if err := ie.store.FailIngest(ie.ctx, docID, "the stored file could not be read"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	if _, _, err := ie.blob.Get(ie.ctx, key); !errors.Is(err, blobstore.ErrNotFound) {
		t.Fatalf("a failed ingest left its stored file behind: %v", err)
	}
	// The ROW stays, with its reason: that is what the uploader reads, and it
	// is the bytes rather than the record that have no further use.
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(docs) != 1 || docs[0].IngestStatus != "failed" {
		t.Fatalf("the failed document is not listed with its reason: %+v", docs)
	}
}

// Two attempts at one document cannot both write its passages.
//
// River dedupes a QUEUED ingest by args, but a worker declared dead while it is
// still running has its row rescued and re-run, so the overlap is reachable.
// Without a guard both attempts delete the previous chunks and insert their
// own: the corpus holds every passage twice, cites duplicates, reports a
// chunk_count half of what it holds, and passes the per-corpus ceiling while
// over it.
func TestTwoAttemptsAtOneDocumentCannotBothWriteItsPassages(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(3))

	// Both attempts read the document before either writes — the interleaving a
	// rescued worker produces.
	first, err := ie.store.BeginIngest(ie.ctx, docID)
	if err != nil {
		t.Fatalf("first begin: %v", err)
	}
	second, err := ie.store.BeginIngest(ie.ctx, docID)
	if err != nil {
		t.Fatalf("second begin: %v", err)
	}
	chunks := knowledge.ChunkText(prose(3))
	if err := ie.store.WriteChunks(ie.ctx, docID, first.CorpusID, chunks); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// The second attempt is REFUSED rather than doubling the corpus. Which
	// error it is does not matter — what matters is that it does not succeed.
	if err := ie.store.WriteChunks(ie.ctx, docID, second.CorpusID, chunks); err == nil {
		t.Fatal("a second attempt wrote the same passages again")
	}
	if live := liveChunkCount(t, ie.env, ie.corpus); live != len(chunks) {
		t.Fatalf("the corpus holds %d passages, want the %d one attempt wrote", live, len(chunks))
	}
}

// The same bytes twice is refused, naming what already holds them.
//
// Re-uploading a file is the most natural repair gesture there is. Accepting it
// would file a SECOND document holding the same passages: they would compete
// for the eight retrieval slots, an answer could cite one sentence twice from
// two apparently different documents, and both copies would count against the
// corpus ceiling.
func TestUploadingTheSameBytesTwiceIsRefusedNamingWhatHoldsThem(t *testing.T) {
	ie := newIngestEnv(t)
	ie.upload(t, "operating.md", "text/markdown", prose(2))

	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "operating-copy.md",
		ContentType: "text/markdown",
		Content:     strings.NewReader(prose(2)),
	}, ie.queue)

	var filed *knowledge.AlreadyFiledError
	if !errors.As(err, &filed) {
		t.Fatalf("re-upload = %v, want AlreadyFiledError", err)
	}
	// It names the document, because "you already uploaded it, it is called X"
	// is the answer the uploader wants.
	if filed.Filename != "operating.md" {
		t.Fatalf("the refusal names %q, want the document that holds the bytes", filed.Filename)
	}
	if n := liveDocumentCount(t, ie.env, ie.corpus); n != 1 {
		t.Fatalf("the corpus holds %d documents after a refused duplicate", n)
	}
}

// A DIFFERENT file with the same name is not a duplicate: the bytes decide, not
// the filename.
func TestADifferentFileWithTheSameNameIsNotADuplicate(t *testing.T) {
	ie := newIngestEnv(t)
	ie.upload(t, "operating.md", "text/markdown", prose(2))
	ie.upload(t, "operating.md", "text/markdown", prose(3))

	if n := liveDocumentCount(t, ie.env, ie.corpus); n != 2 {
		t.Fatalf("the corpus holds %d documents, want both files", n)
	}
}

// The ceiling is a statement about the CORPUS, so it must hold across two
// documents ingesting at once — not only across two attempts at one.
func TestTheCeilingHoldsAcrossTwoDocumentsInOneCorpus(t *testing.T) {
	ie := newIngestEnv(t)
	filler := ie.upload(t, "filler.md", "text/markdown", prose(1))
	// Room for one of the two documents and not both, so the ceiling is what
	// decides rather than either document's own size.
	fillToPassageCeilingLess(t, ie.env, ie.corpus, filler, len(knowledge.ChunkText(prose(2)))+1)

	// Distinct bytes, or the duplicate refusal fires before the ceiling does.
	first := ie.upload(t, "a.md", "text/markdown", prose(2))
	second := ie.upload(t, "b.md", "text/markdown", prose(3))
	firstSrc, err := ie.store.BeginIngest(ie.ctx, first)
	if err != nil {
		t.Fatalf("begin first: %v", err)
	}
	secondSrc, err := ie.store.BeginIngest(ie.ctx, second)
	if err != nil {
		t.Fatalf("begin second: %v", err)
	}
	if err := ie.store.WriteChunks(ie.ctx, first, firstSrc.CorpusID, knowledge.ChunkText(prose(2))); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// The second is refused: the corpus is full, whichever document is asking.
	var full *knowledge.CorpusFullError
	if err := ie.store.WriteChunks(ie.ctx, second, secondSrc.CorpusID, knowledge.ChunkText(prose(3))); !errors.As(err, &full) {
		t.Fatalf("second write = %v, want CorpusFullError", err)
	}
	if live := liveChunkCount(t, ie.env, ie.corpus); live > knowledgePassageCeiling {
		t.Fatalf("the corpus holds %d passages, past its %d ceiling", live, knowledgePassageCeiling)
	}
}

func fillToPassageCeilingLess(t *testing.T, e *Env, corpusID, documentID ids.UUID, short int) {
	t.Helper()
	e.WsExec(t,
		`INSERT INTO knowledge_chunk (corpus_id, document_id, chunk_ix, text, chunk_hash)
		 SELECT $1, $2, g, 'filler ' || g, md5(g::text)
		 FROM generate_series(1, $3) AS g`,
		corpusID, documentID, knowledgePassageCeiling-short)
}

// A document whose ingest stopped without saying so is closed by the sweep.
//
// River rescues a job whose worker died, but a process that dies on the LAST
// attempt leaves nothing to rescue and no attempt to run. The document then
// sits `running` forever, readiness reads it as in-flight, and the WHOLE corpus
// answers not_ready permanently — every other document in it unaskable because
// one upload's worker was killed.
func TestAnIngestThatStoppedWithoutFinishingIsClosedBytheSweep(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(2))
	if _, err := ie.store.BeginIngest(ie.ctx, docID); err != nil {
		t.Fatalf("begin: %v", err)
	}
	// Backdated rather than waited for: the threshold is compared against the
	// database's clock, so a test moves the row rather than the clock. It moves
	// ingest_started_at and NOT updated_at — the trigger owns updated_at and
	// would overwrite a backdate with now() on the very statement that set it.
	ie.env.WsExec(t,
		`UPDATE knowledge_document SET ingest_started_at = now() - interval '3 hours' WHERE id = $1`, docID)

	closed, err := ie.store.SweepAbandonedIngests(ie.ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if closed != 1 {
		t.Fatalf("the sweep closed %d ingests, want 1", closed)
	}
	docs, err := ie.store.ListDocuments(ie.ctx, ie.corpus)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if docs[0].IngestStatus != "failed" {
		t.Fatalf("the abandoned document is %q, want failed", docs[0].IngestStatus)
	}
	// With a reason the uploader can act on, rather than a bare state.
	if docs[0].IngestDetail == nil || !strings.Contains(*docs[0].IngestDetail, "upload it again") {
		t.Fatalf("the abandoned document's reason is %v", docs[0].IngestDetail)
	}
}

// A document still INSIDE its wall clock is slow, not gone, and the sweep must
// leave it alone. Closing it would fail an ingest that is about to succeed.
func TestASweepLeavesAnIngestThatIsMerelySlowAlone(t *testing.T) {
	ie := newIngestEnv(t)
	docID := ie.upload(t, "operating.md", "text/markdown", prose(2))
	if _, err := ie.store.BeginIngest(ie.ctx, docID); err != nil {
		t.Fatalf("begin: %v", err)
	}

	closed, err := ie.store.SweepAbandonedIngests(ie.ctx)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if closed != 0 {
		t.Fatalf("the sweep closed %d ingests that had only just started", closed)
	}
}

// A client that sent no type at all is read by its filename.
//
// A platform with no registry entry for `.md` makes the browser report an EMPTY
// content type, and this route's flagship file would otherwise be refused with
// "` ` cannot be read as text" — a refusal naming nothing, for exactly the file
// the route wants. A curl caller omitting the part header hits the same wall.
func TestAnUploadWithNoContentTypeIsReadByItsFilename(t *testing.T) {
	ie := newIngestEnv(t)
	for _, c := range []struct{ filename, want string }{
		{"handbook.md", "text/markdown"},
		{"notes.txt", "text/plain"},
		{"rows.csv", "text/csv"},
		{"config.json", "application/json"},
	} {
		doc, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
			CorpusID:    ie.corpus,
			Filename:    c.filename,
			ContentType: "",
			Content:     strings.NewReader(prose(1) + c.filename),
		}, ie.queue)
		if err != nil {
			t.Fatalf("upload %s with no content type: %v", c.filename, err)
		}
		if doc.ContentType != c.want {
			t.Fatalf("%s was filed as %q, want %q", c.filename, doc.ContentType, c.want)
		}
	}
}

// A STATED type stands, wrong extension or not: it is the client's claim, and
// a filename is only a name.
func TestAStatedContentTypeIsNotOverriddenByTheExtension(t *testing.T) {
	ie := newIngestEnv(t)
	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "handbook.md",
		ContentType: "application/pdf",
		Content:     strings.NewReader("%PDF-1.7"),
	}, ie.queue)

	var unsupported *knowledge.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("a PDF named .md = %v, want UnsupportedTypeError", err)
	}
}

// And an extension this route does not read is still refused, so the fallback
// widens nothing.
func TestAnUploadWithNoContentTypeAndAnUnreadableExtensionIsRefused(t *testing.T) {
	ie := newIngestEnv(t)
	_, err := ie.store.UploadDocument(ie.ctx, knowledge.NewDocument{
		CorpusID:    ie.corpus,
		Filename:    "handbook.docx",
		ContentType: "",
		Content:     strings.NewReader("PK\x03\x04"),
	}, ie.queue)

	var unsupported *knowledge.UnsupportedTypeError
	if !errors.As(err, &unsupported) {
		t.Fatalf("a .docx with no stated type = %v, want UnsupportedTypeError", err)
	}
}
