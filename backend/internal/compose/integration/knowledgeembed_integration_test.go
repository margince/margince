// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The embed suite: what costs a model call and what does not, and the two
// vectors that must never reach storage — the zero one, and one written apart
// from the identity that produced it.

import (
	"context"
	"crypto/sha256"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/model"
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
	// zeroFor makes it answer that vector only for inputs equal to this string,
	// so a fixture can make ONE document unembeddable while its neighbours
	// succeed.
	zeroFor string
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
		if !e.zero && req.Inputs[i] != e.zeroFor {
			// Derived from a HASH of the input rather than its length. Length
			// alone gave two passages of the same size the identical vector —
			// which is the ordinary case for chunked prose, since every span
			// but the last is cut to the same ceiling — so a test that meant to
			// distinguish two passages was comparing one vector with itself.
			sum := sha256.Sum256([]byte(req.Inputs[i]))
			for d := range vec {
				// +1 keeps every component non-zero, which is what stops the
				// zero-vector guard firing on a fixture that did not mean to
				// exercise it.
				vec[d] = float32(int(sum[d%len(sum)])%7+1) / 10
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

// A passage whose TEXT changed under an in-flight model call does not get that
// call's vector stamped onto it.
//
// The row is edited in place — same id, new text, new hash — because that is
// what isolates the guard. An earlier version of this test replaced the rows
// through WriteChunks, which DELETES them: the assertion then held whether or
// not storeVectors compared chunk_hash at all, and removing the guard left it
// green. It was a test named for a CAS that never touched one.
func TestAVectorIsNotStampedOntoAPassageThatChangedUnderIt(t *testing.T) {
	ee := newEmbedEnv(t)
	// The passages as they stand, and what they will be embedded against.
	pending := ee.env.WsCount(t,
		`SELECT count(*) FROM knowledge_chunk WHERE corpus_id = $1 AND embed_identity IS NULL`, ee.corpus)
	if pending == 0 {
		t.Fatal("nothing was pending, so the edit below would prove nothing")
	}

	// The text moves under the pass: EmbedDocument reads the rows and their
	// hashes, and the row is edited between that read and the write. Simulated
	// by embedding, then editing, then embedding again under the SAME identity
	// — the second pass finds a hash it has never seen and must not carry the
	// first pass's stamp onto it.
	if _, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	ee.env.WsExec(t,
		`UPDATE knowledge_chunk
		    SET text = 'entirely different prose', chunk_hash = 'a-hash-nothing-was-embedded-under'
		  WHERE corpus_id = $1`, ee.corpus)

	// Every row now carries a vector computed for text it no longer holds, and
	// the identity says so. The next pass re-embeds them because the hash moved
	// — which is the OTHER half of the same guard.
	n, err := ee.store.EmbedDocument(ee.ctx, ee.doc, ee.embedder)
	if err != nil {
		t.Fatalf("second embed: %v", err)
	}
	if n == 0 {
		t.Fatal("a passage whose text changed was treated as already current")
	}
}
