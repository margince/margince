// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The embed suite: what costs a model call and what does not, and the two
// vectors that must never reach storage — the zero one, and one written apart
// from the identity that produced it.

import (
	"context"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// countingEmbedder is the embed lane, counting what it was asked to compute.
//
// It answers a deterministic non-zero vector derived from the input, so two
// different passages get two different vectors and an assertion about WHICH
// passage a vector belongs to is possible. Its identity is settable, because a
// binding swap is half of what this suite is about.
type countingEmbedder struct {
	identity string
	dims     int
	calls    int
	inputs   int
	// zero makes it answer the vector that must never be stored.
	zero bool
	// short makes it answer fewer vectors than it was given inputs.
	short bool
}

func newCountingEmbedder() *countingEmbedder {
	return &countingEmbedder{identity: "fake/embed@8", dims: 8}
}

func (e *countingEmbedder) EmbedIdentity() (string, int) { return e.identity, e.dims }

func (e *countingEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	e.calls++
	e.inputs += len(req.Inputs)
	n := len(req.Inputs)
	if e.short && n > 1 {
		n--
	}
	vectors := make([][]float32, n)
	for i := range vectors {
		vec := make([]float32, e.dims)
		if !e.zero {
			for d := range vec {
				// Derived from the input so different text gives different
				// vectors; the +1 keeps every component non-zero.
				vec[d] = float32(len(req.Inputs[i])%7+d+1) / 10
			}
		}
		vectors[i] = vec
	}
	return model.Embeddings{Vectors: vectors, Dims: e.dims}, nil
}

// embedEnv is an ingested document waiting for its vectors.
type embedEnv struct {
	*ingestEnv
	doc      ids.UUID
	embedder *countingEmbedder
}

func newEmbedEnv(t *testing.T) *embedEnv {
	t.Helper()
	ie := newIngestEnv(t)
	ee := &embedEnv{ingestEnv: ie, embedder: newCountingEmbedder()}
	ee.doc = ie.upload(t, "operating.md", "text/markdown", prose(2))
	ie.ingest(t, ee.doc)
	return ee
}

func embeddedCount(t *testing.T, e *Env, corpusID ids.UUID) int {
	t.Helper()
	return e.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk
		 WHERE corpus_id = $1 AND archived_at IS NULL AND embed_identity IS NOT NULL`, corpusID)
}

// The first pass embeds every passage; the corpus's coverage then says so.
func TestEmbeddingADocumentGivesEveryPassageAVector(t *testing.T) {
	ee := newEmbedEnv(t)

	n, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder)
	if err != nil {
		t.Fatalf("embed: %v", err)
	}
	total := liveChunkCount(t, ee.env, ee.corpus)
	if n != total {
		t.Fatalf("embedded %d of %d passages", n, total)
	}
	if got := embeddedCount(t, ee.env, ee.corpus); got != total {
		t.Fatalf("%d passages carry a vector, want %d", got, total)
	}
	corpus, err := ee.store.ReadCorpus(ee.ctx, ee.corpus)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	if corpus.Coverage.ChunksEmbedded != total || corpus.Coverage.ChunksTotal != total {
		t.Fatalf("coverage = %+v, want %d of %d embedded", corpus.Coverage, total, total)
	}
}

// Re-embedding an unchanged document under an unchanged binding costs no model
// calls. This is what makes the re-embed sweep resumable for free.
func TestEmbeddingAnAlreadyCurrentDocumentCostsNoCalls(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	callsAfterFirst := ee.embedder.calls
	if callsAfterFirst == 0 {
		t.Fatal("the first pass made no model call, so the second proving nothing would prove nothing")
	}

	n, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder)
	if err != nil {
		t.Fatalf("second embed: %v", err)
	}
	if n != 0 {
		t.Fatalf("the second pass embedded %d passages, want 0", n)
	}
	if ee.embedder.calls != callsAfterFirst {
		t.Fatalf("the second pass made %d more model call(s)", ee.embedder.calls-callsAfterFirst)
	}
}

// A binding swap re-embeds even though the text is identical: the stored vector
// lives in a space the live query no longer shares, and leaving it stamped with
// a model that no longer serves the workspace makes it indistinguishable from a
// live row.
func TestAChangedIdentityReEmbedsIdenticalText(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	total := liveChunkCount(t, ee.env, ee.corpus)

	ee.embedder.identity = "fake/other@8"
	n, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder)
	if err != nil {
		t.Fatalf("embed under the new binding: %v", err)
	}
	if n != total {
		t.Fatalf("a binding swap re-embedded %d of %d passages", n, total)
	}
	// And the rows now say so, or the ask's identity filter would exclude
	// every passage in the corpus.
	if got := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk WHERE corpus_id = $1 AND embed_identity = 'fake/other@8'`,
		ee.corpus); got != total {
		t.Fatalf("%d passages carry the new identity, want %d", got, total)
	}
}

// A zero vector never reaches storage — cosine against it is NaN and
// `ORDER BY sim DESC` sorts NaN first, so one stored zero silently outranks
// every real passage in the corpus.
func TestAZeroVectorIsRefusedAtWrite(t *testing.T) {
	ee := newEmbedEnv(t)
	ee.embedder.zero = true

	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err == nil {
		t.Fatal("a zero vector was accepted")
	}
	if got := embeddedCount(t, ee.env, ee.corpus); got != 0 {
		t.Fatalf("%d zero vector(s) reached storage", got)
	}
}

// A provider answering fewer vectors than it was given inputs is refused before
// anything is written. Writing them positionally would stamp each answer onto
// the wrong passage — a corpus that cites the wrong document rather than one
// that fails.
func TestAShortEmbedAnswerIsRefusedBeforeAnythingIsWritten(t *testing.T) {
	ee := newEmbedEnv(t)
	ee.embedder.short = true

	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err == nil {
		t.Fatal("an embed answer short of its inputs was accepted")
	}
	if got := embeddedCount(t, ee.env, ee.corpus); got != 0 {
		t.Fatalf("%d passage(s) were stamped from a short answer", got)
	}
}

// With no embed lane bound, embedding is a no-op rather than an error: an
// installation configured without one is a legitimate deployment shape, and the
// ask reports retrieval_unavailable rather than treating it as a fault here.
func TestAnUnboundEmbedLaneEmbedsNothingAndDoesNotFail(t *testing.T) {
	ee := newEmbedEnv(t)
	ee.embedder.identity = ""

	n, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder)
	if err != nil {
		t.Fatalf("embedding with no lane bound: %v", err)
	}
	if n != 0 {
		t.Fatalf("embedded %d passages with no lane bound", n)
	}
	// It did not even ask: the identity check runs before the read, so an
	// unbound installation pays no database round trip per document either.
	if ee.embedder.calls != 0 {
		t.Fatalf("%d model call(s) with no lane bound", ee.embedder.calls)
	}
}

// The vector and the identity that produced it are written together or not at
// all. A row carrying one without the other is retrievable and unrankable,
// which is worse than an unembedded row — the schema refuses it, and this is
// the test that would notice the write splitting in two.
func TestNoPassageEverCarriesAnIdentityWithoutAVector(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if got := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk
		 WHERE corpus_id = $1 AND (embed_identity IS NULL) <> (embedding IS NULL)`,
		ee.corpus); got != 0 {
		t.Fatalf("%d passage(s) carry an identity and no vector, or the reverse", got)
	}
}

// A re-ingest that replaced a passage while the model call was in flight must
// not have this vector stamped onto it: the row's text is no longer the text
// that was embedded.
func TestAVectorIsNotStampedOntoAPassageThatChangedUnderIt(t *testing.T) {
	ee := newEmbedEnv(t)
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	// The passages this document had are replaced by a fresh attempt, exactly
	// as a re-ingest replaces them.
	before := embeddedCount(t, ee.env, ee.corpus)
	if before == 0 {
		t.Fatal("nothing was embedded, so the replacement below would prove nothing")
	}
	src, err := ee.store.BeginIngest(ee.ctx, ee.doc)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if err := ee.store.WriteChunks(ee.ctx, ee.doc, src.CorpusID,
		knowledge.ChunkText(strings.Repeat("Entirely different prose about something else. ", 40))); err != nil {
		t.Fatalf("rewrite chunks: %v", err)
	}
	// The old rows are gone with their vectors; the new ones have none until
	// they are embedded in their own right.
	if got := embeddedCount(t, ee.env, ee.corpus); got != 0 {
		t.Fatalf("%d passage(s) kept a vector across a re-ingest", got)
	}
}
