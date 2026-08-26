// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The drift-repair suite: what happens to a corpus when the embed binding
// moves under it.
//
// The failure this prevents is quiet and permanent. A swapped binding leaves
// every stored vector in a space the live query no longer shares, the ask's
// identity filter excludes all of them, and the corpus reports not_ready
// forever — while re-uploading the same file does nothing, because the checksum
// matches and nothing re-chunks.

import (
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
)

func staleVectorCount(t *testing.T, e *Env, corpusID ids.UUID, identity string) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk
		 WHERE corpus_id = $1 AND archived_at IS NULL AND embed_identity IS DISTINCT FROM $2`,
		corpusID, identity)
}

func reindexingNow(t *testing.T, e *Env, corpusID ids.UUID) bool {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM knowledge_corpus WHERE id = $1 AND reindexing`, corpusID) == 1
}

// A binding swap must not brick a corpus.
func TestASwappedBindingIsRepairedByTheDriftSweep(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	total := liveChunkCount(t, ee.env, ee.corpus)

	ee.embedder.identity = "fake/other@8"
	if stale := staleVectorCount(t, ee.env, ee.corpus, ee.embedder.identity); stale != total {
		t.Fatalf("%d of %d passages are stale after the swap, want all of them", stale, total)
	}

	repaired, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if repaired != total {
		t.Fatalf("the sweep repaired %d of %d passages", repaired, total)
	}
	if stale := staleVectorCount(t, ee.env, ee.corpus, ee.embedder.identity); stale != 0 {
		t.Fatalf("%d passage(s) still carry the superseded binding", stale)
	}
}

// The flag is lowered on the way out. One left raised makes a corpus that has
// stopped being repaired look like one still being repaired, and nothing would
// ever lower it.
func TestTheDriftSweepLeavesNoCorpusMarkedReindexing(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	ee.embedder.identity = "fake/other@8"
	if _, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder); err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if reindexingNow(t, ee.env, ee.corpus) {
		t.Fatal("the corpus is still marked reindexing after the sweep finished")
	}
}

// Even when the sweep FAILS. A crashed repair must not leave the flag raised.
func TestAFailedDriftSweepStillLowersTheFlag(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	ee.embedder.identity = "fake/other@8"
	// The lane now answers the one vector that may never be stored, so the
	// repair fails partway rather than by refusing to start.
	ee.embedder.zero = true

	if _, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder); err == nil {
		t.Fatal("a sweep that could not embed reported success")
	}
	if reindexingNow(t, ee.env, ee.corpus) {
		t.Fatal("a failed sweep left the corpus marked reindexing")
	}
}

// Running it twice costs nothing the second time: the sweep is resumable for
// free, which is what makes it safe to run on a period.
func TestASecondDriftSweepCostsNoModelCalls(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	ee.embedder.identity = "fake/other@8"
	if _, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	callsAfterRepair := ee.embedder.calls

	repaired, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if repaired != 0 {
		t.Fatalf("the second sweep repaired %d passages, want 0", repaired)
	}
	if ee.embedder.calls != callsAfterRepair {
		t.Fatalf("the second sweep made %d more model call(s)", ee.embedder.calls-callsAfterRepair)
	}
}

// Passages an ingest wrote before any lane was bound are repaired by the same
// pass. An installation that gained an embed lane after its documents landed
// would otherwise stay unanswerable in a second, separate way.
func TestPassagesIngestedWithNoLaneAreEmbeddedByTheSweep(t *testing.T) {
	ie := newIngestEnv(t)
	doc := ie.upload(t, "operating.md", "text/markdown", prose(2))
	ie.ingest(t, doc) // no embed lane involved: the passages land vectorless
	total := liveChunkCount(t, ie.env, ie.corpus)
	if embeddedCount(t, ie.env, ie.corpus) != 0 {
		t.Fatal("the fixture embedded something, so the sweep below would prove nothing")
	}

	e := newCountingEmbedder()
	repaired, err := ie.store.SweepCorpusDrift(ie.ctx, e)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if repaired != total {
		t.Fatalf("the sweep embedded %d of %d never-embedded passages", repaired, total)
	}
}

// With no lane bound the sweep is a no-op, not a repair into nothing: there is
// no live space to have drifted from.
func TestTheDriftSweepWithNoLaneBoundDoesNothing(t *testing.T) {
	ee := newEmbedEnv(t)
	ee.embedder.identity = ""

	repaired, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder)
	if err != nil {
		t.Fatalf("sweep with no lane bound: %v", err)
	}
	if repaired != 0 || ee.embedder.calls != 0 {
		t.Fatalf("repaired %d passages in %d call(s) with no lane bound", repaired, ee.embedder.calls)
	}
}

// An archived corpus is not repaired. Spending model budget re-embedding what
// nobody can ask is the sweep paying for its own irrelevance.
func TestTheDriftSweepSkipsAnArchivedCorpus(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	if err := ee.store.ArchiveCorpus(ee.ctx, ee.corpus); err != nil {
		t.Fatalf("archive: %v", err)
	}
	ee.embedder.identity = "fake/other@8"
	callsBefore := ee.embedder.calls

	repaired, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if repaired != 0 || ee.embedder.calls != callsBefore {
		t.Fatalf("an archived corpus was re-embedded: %d passages, %d call(s)",
			repaired, ee.embedder.calls-callsBefore)
	}
}

// A document whose ingest failed is not swept: it has no live passages, and a
// document still running gets its vectors from the attempt itself.
func TestTheDriftSweepSkipsADocumentThatFailedIngest(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	if err := ee.store.FailIngest(ee.ctx, ee.doc, "the stored file could not be read"); err != nil {
		t.Fatalf("fail: %v", err)
	}
	ee.embedder.identity = "fake/other@8"
	callsBefore := ee.embedder.calls

	repaired, err := ee.store.SweepCorpusDrift(ee.ctx, ee.embedder)
	if err != nil {
		t.Fatalf("sweep: %v", err)
	}
	if repaired != 0 || ee.embedder.calls != callsBefore {
		t.Fatalf("a failed document was re-embedded: %d passages, %d call(s)",
			repaired, ee.embedder.calls-callsBefore)
	}
}

// One document the sweep cannot repair must not block every document behind it.
//
// The sweep is periodic and its document order is stable, so returning at the
// first failure stopped it at the SAME document every pass — one passage a
// provider will not embed left every later document in the workspace under a
// superseded binding forever, unaskable, with nothing in the corpus to explain
// it. The pass now continues and reports what failed.
func TestOneUnrepairableDocumentDoesNotBlockTheRest(t *testing.T) {
	ie := newIngestEnv(t)
	e := newCountingEmbedder()

	first := ie.upload(t, "first.md", "text/markdown", prose(2))
	second := ie.upload(t, "second.md", "text/markdown", prose(3))
	ie.ingest(t, first)
	ie.ingest(t, second)
	total := liveChunkCount(t, ie.env, ie.corpus)

	// One document's passages are given a hash the writer never produced, which
	// is enough to keep them selected for embedding forever; the lane then
	// refuses THEM specifically by answering a zero vector for their text.
	if _, err := ie.store.SweepCorpusDrift(ie.ctx, e); err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if embeddedCount(t, ie.env, ie.corpus) != total {
		t.Fatal("the fixture did not embed everything, so the failure below would prove nothing")
	}

	// The FIRST document by id is broken, which is the one the sweep reaches
	// first — the sweep orders by document id precisely so this is decidable.
	// Breaking the later one instead would let the old, early-returning code
	// pass half the time: it would repair the good document, hit the bad one,
	// and return with a non-zero count.
	ie.env.WsExec(t, `UPDATE knowledge_chunk SET text = '' WHERE document_id = $1`, first)
	e.identity = "fake/other@8"
	e.zeroFor = ""

	repaired, err := ie.store.SweepCorpusDrift(ie.ctx, e)
	if err == nil {
		t.Fatal("a sweep that could not repair a document reported success")
	}
	// The OTHER document was repaired anyway, which is the whole point.
	if repaired == 0 {
		t.Fatal("the sweep repaired nothing: one bad document blocked the rest")
	}
}
