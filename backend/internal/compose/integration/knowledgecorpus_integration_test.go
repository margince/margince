// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The corpus CRUD suite: the grounding floor a corpus is born with, the single
// default the palette asks, the coverage counts a screen reads, the archive
// that takes the documents and their chunks with it, and the admin-writes /
// everyone-reads posture the RBAC migration seeds.

import (
	"errors"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// corpusAdminPerms mirrors the admin/ops grant the RBAC migration seeds.
var corpusAdminPerms = principal.Permissions{
	RoleKeys: []string{"admin"},
	Objects: map[string]principal.ObjectGrant{
		"knowledge_corpus":   {Create: true, Read: true, Update: true, Delete: true},
		"knowledge_document": {Create: true, Read: true, Update: true, Delete: true},
	},
	RowScope: principal.RowScopeAll,
}

// corpusRepPerms mirrors the manager/rep/read_only grant: the ask, and nothing
// that moves the floor it answers under.
var corpusRepPerms = principal.Permissions{
	RoleKeys: []string{"rep"},
	Objects: map[string]principal.ObjectGrant{
		"knowledge_corpus":   {Read: true},
		"knowledge_document": {Read: true},
	},
	RowScope: principal.RowScopeTeam,
}

func howTo(name string) knowledge.NewCorpus {
	return knowledge.NewCorpus{Name: name, TopicStatement: "How this product is operated."}
}

func TestCreatingACorpusStampsTheDefaultFloor(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	got, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.MinSimilarity != knowledge.DefaultMinSimilarity {
		t.Fatalf("floor = %v, want the default %v", got.MinSimilarity, knowledge.DefaultMinSimilarity)
	}
	// A corpus is born empty, and says so: the screen reads these three before
	// any document exists, and a nil coverage would render as a blank rather
	// than as a zero.
	if got.Coverage.DocumentsTotal != 0 || got.Coverage.ChunksTotal != 0 || got.Coverage.ChunksEmbedded != 0 {
		t.Fatalf("a new corpus must report zero coverage, got %+v", got.Coverage)
	}
}

// An explicit floor is the caller's, not the default's — the transport passes
// it through rather than defaulting, so the two cases stay distinguishable.
func TestAnExplicitFloorSurvivesTheCreate(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	floor := 0.62
	in := howTo("Handbook")
	in.MinSimilarity = &floor
	got, err := store.CreateCorpus(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.MinSimilarity != floor {
		t.Fatalf("floor = %v, want the caller's %v", got.MinSimilarity, floor)
	}
}

// Two defaults is a palette that asks a different corpus depending on row
// order, so the second one moves the flag rather than joining it.
func TestASecondDefaultCorpusTakesTheFlagFromTheFirst(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	firstIn := howTo("A")
	firstIn.DefaultAsk = true
	first, err := store.CreateCorpus(ctx, firstIn)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	secondIn := howTo("B")
	secondIn.DefaultAsk = true
	second, err := store.CreateCorpus(ctx, secondIn)
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	reread, err := store.ReadCorpus(ctx, ids.UUID(first.Id))
	if err != nil {
		t.Fatalf("reread first: %v", err)
	}
	if reread.DefaultAsk {
		t.Fatal("the first corpus kept the default flag; the second should have taken it")
	}
	if !second.DefaultAsk {
		t.Fatal("the second corpus did not take the default flag")
	}
}

// The same rule on the edit path: raising the flag by patch moves it too, and
// the partial unique index is never the thing that reports the collision.
func TestPatchingTheDefaultFlagMovesItRatherThanColliding(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	firstIn := howTo("A")
	firstIn.DefaultAsk = true
	first, err := store.CreateCorpus(ctx, firstIn)
	if err != nil {
		t.Fatalf("create first: %v", err)
	}
	second, err := store.CreateCorpus(ctx, howTo("B"))
	if err != nil {
		t.Fatalf("create second: %v", err)
	}
	on := true
	promoted, err := store.EditCorpus(ctx, ids.UUID(second.Id), knowledge.UpdateCorpus{DefaultAsk: &on})
	if err != nil {
		t.Fatalf("patch the second corpus to default: %v", err)
	}
	if !promoted.DefaultAsk {
		t.Fatal("the patched corpus did not become the default")
	}
	demoted, err := store.ReadCorpus(ctx, ids.UUID(first.Id))
	if err != nil {
		t.Fatalf("reread first: %v", err)
	}
	if demoted.DefaultAsk {
		t.Fatal("the previous default kept its flag")
	}
}

// An empty patch is a read, not a write: no audit row, and the same corpus
// back. A caller that PATCHes nothing has changed nothing.
func TestAnEmptyPatchWritesNothing(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	before := corpusAuditRows(t, e, ids.UUID(made.Id))
	if _, err := store.EditCorpus(ctx, ids.UUID(made.Id), knowledge.UpdateCorpus{}); err != nil {
		t.Fatalf("empty patch: %v", err)
	}
	if after := corpusAuditRows(t, e, ids.UUID(made.Id)); after != before {
		t.Fatalf("an empty patch wrote %d audit row(s)", after-before)
	}
}

// Archiving a corpus takes its documents and their chunks with it, in one
// transaction. A chunk left live under an archived corpus stays retrievable,
// and would be cited by name out of a corpus the screen no longer shows.
func TestArchivingACorpusTakesItsDocumentsAndChunks(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	corpusID := ids.UUID(made.Id)
	seedDocumentWithChunk(t, e, corpusID)

	if err := store.ArchiveCorpus(ctx, corpusID); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if live := liveChunkCount(t, e, corpusID); live != 0 {
		t.Fatalf("%d chunk(s) survived the corpus archive", live)
	}
	if live := liveDocumentCount(t, e, corpusID); live != 0 {
		t.Fatalf("%d document(s) survived the corpus archive", live)
	}
	if _, err := store.ReadCorpus(ctx, corpusID); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading an archived corpus = %v, want ErrNotFound", err)
	}
}

// A repeat archive is the same answer and no second audit row — the caller who
// retries a 204 has not archived it twice.
func TestArchivingACorpusTwiceWritesOneAuditRow(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	made, err := store.CreateCorpus(ctx, howTo("How-to"))
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	corpusID := ids.UUID(made.Id)
	if err := store.ArchiveCorpus(ctx, corpusID); err != nil {
		t.Fatalf("first archive: %v", err)
	}
	after := corpusAuditRows(t, e, corpusID)
	if err := store.ArchiveCorpus(ctx, corpusID); err != nil {
		t.Fatalf("second archive: %v", err)
	}
	if again := corpusAuditRows(t, e, corpusID); again != after {
		t.Fatalf("the second archive wrote %d more audit row(s)", again-after)
	}
}

// An archived corpus leaves the list. The screen's list is the live set.
func TestAnArchivedCorpusLeavesTheList(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	kept, err := store.CreateCorpus(ctx, howTo("Kept"))
	if err != nil {
		t.Fatalf("create kept: %v", err)
	}
	gone, err := store.CreateCorpus(ctx, howTo("Gone"))
	if err != nil {
		t.Fatalf("create gone: %v", err)
	}
	if err := store.ArchiveCorpus(ctx, ids.UUID(gone.Id)); err != nil {
		t.Fatalf("archive: %v", err)
	}
	list, err := store.ListCorpora(ctx)
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || list[0].Id != kept.Id {
		t.Fatalf("list = %d corpora, want only the live one", len(list))
	}
}

// The seeded posture: a rep asks, and cannot move the floor the answer is
// refused under. Object denial is 403, never a silent no-op.
func TestARepMayReadACorpusAndMayNotDefineOne(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	admin := e.As(e.Rep1, nil, corpusAdminPerms)
	rep := e.As(e.Rep1, nil, corpusRepPerms)

	made, err := store.CreateCorpus(admin, howTo("How-to"))
	if err != nil {
		t.Fatalf("admin create: %v", err)
	}
	if _, err := store.ReadCorpus(rep, ids.UUID(made.Id)); err != nil {
		t.Fatalf("a rep must be able to read a corpus: %v", err)
	}
	if _, err := store.CreateCorpus(rep, howTo("Rep's own")); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep create = %v, want ErrPermissionDenied", err)
	}
	floor := 0.9
	if _, err := store.EditCorpus(rep, ids.UUID(made.Id), knowledge.UpdateCorpus{MinSimilarity: &floor}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep patch = %v, want ErrPermissionDenied", err)
	}
	if err := store.ArchiveCorpus(rep, ids.UUID(made.Id)); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("rep archive = %v, want ErrPermissionDenied", err)
	}
}

// A corpus that does not exist is absent, not forbidden.
func TestReadingAMissingCorpusIsNotFound(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	ctx := e.As(e.Rep1, nil, corpusAdminPerms)

	if _, err := store.ReadCorpus(ctx, ids.NewV7()); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("read of an unknown id = %v, want ErrNotFound", err)
	}
}

// seedDocumentWithChunk files one document and one chunk directly, because the
// ingest that would write them is not this task's subject: the archive cascade
// is what is under test, not the writer.
func seedDocumentWithChunk(t *testing.T, e *Env, corpusID ids.UUID) {
	t.Helper()
	docID, chunkID := ids.NewV7(), ids.NewV7()
	e.WsExec(t,
		`INSERT INTO knowledge_document (id, corpus_id, filename, content_type, byte_size, storage_key, checksum, captured_by)
		 VALUES ($1, $2, 'operating.md', 'text/markdown', 12, 'k/1', 'sha', 'human:test')`,
		docID, corpusID)
	e.WsExec(t,
		`INSERT INTO knowledge_chunk (id, corpus_id, document_id, chunk_ix, text, chunk_hash)
		 VALUES ($1, $2, $3, 0, 'the operating note', 'h')`,
		chunkID, corpusID, docID)
}

func liveChunkCount(t *testing.T, e *Env, corpusID ids.UUID) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk WHERE corpus_id = $1 AND archived_at IS NULL`, corpusID)
}

func liveDocumentCount(t *testing.T, e *Env, corpusID ids.UUID) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM knowledge_document WHERE corpus_id = $1 AND archived_at IS NULL`, corpusID)
}

func corpusAuditRows(t *testing.T, e *Env, corpusID ids.UUID) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'knowledge_corpus' AND entity_id = $1`, corpusID)
}
