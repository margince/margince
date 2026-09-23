// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// THE MIRROR GATE.
//
// knowledge.RankInMemory is a second implementation of the ranking
// Store.Retrieve does in SQL. It exists so the corpus_ask evaluation loop
// (`worker aitask retrieve`) can run against a bare checkout with no Postgres,
// and a second implementation of one decision is a hazard: a copy can go green
// while the original is broken, and every scenario captured through the copy
// would then be measuring something production does not do.
//
// This is what makes the copy acceptable. It ingests one corpus through the
// real ingest path, embeds it through the real store, and runs BOTH rankers over
// the same question — asserting the same passages in the same order, on both
// sides of the floor and on both sides of the limit.
//
// WHEN THIS GOES RED, the eval loop and production disagree about which
// passages an answer may rest on. The fix is never to change this test's
// expectation: it is to make RankInMemory agree with rankIn again, or to delete
// the mirror.

import (
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// mirrorEnv is a corpus of single-chunk documents, each placed at a similarity
// the test chose.
//
// One chunk per document because the assertion is about ORDER, and a document
// the chunker split would make the expected order depend on where it split.
// Placed by the steered embedder rather than by prose, because real prose lands
// where the model puts it and a gate that hoped for an ordering would be testing
// its own luck.
type mirrorEnv struct {
	*ingestEnv
	embedder *steeredEmbedder
	corpusIn []knowledge.EmbeddedPassage
}

// mirrorPassageCount is deliberately above knowledge.RetrieveLimit, so the gate
// covers what the limit CUTS as well as what it keeps. An equal count would let
// a ranker that ignored the limit pass.
const mirrorPassageCount = knowledge.RetrieveLimit + 4

func newMirrorEnv(t *testing.T) *mirrorEnv {
	t.Helper()
	ie := newIngestEnv(t)
	me := &mirrorEnv{ingestEnv: ie, embedder: newSteeredEmbedder()}
	// The question sits on the first axis. Each passage is tilted away from it
	// by a different amount, so every pairwise similarity differs and an order
	// is a real assertion rather than a tie broken two ways.
	me.embedder.vectors["which passage is closest"] = []float32{1, 0, 0, 0}
	var docs []ids.UUID
	for i := range mirrorPassageCount {
		text := fmt.Sprintf("Passage %02d: the corpus answers only from what a workspace filed in it.", i)
		name := fmt.Sprintf("page%02d.md", i)
		// Tilt grows with i, so passage 0 is nearest and the last is furthest —
		// and the ranking is therefore NOT the ingest order, which is what a
		// ranker that forgot to sort would silently return.
		vector := []float32{1, float32(i) * 0.35, 0, 0}
		me.embedder.vectors[text] = vector

		doc := ie.upload(t, name, "text/markdown", text)
		ie.ingest(t, doc)
		docs = append(docs, doc)
		me.corpusIn = append(me.corpusIn, knowledge.EmbeddedPassage{
			DocumentName: name,
			Chunk:        knowledge.Chunk{Text: text, StartLine: 1},
			Vector:       vector,
		})
	}
	// Through the store's own EmbedDocument, not a hand-written insert: the ask
	// filters on the embed identity that writer stamps, and a test that wrote
	// its own rows would be proving something about its own SQL.
	for _, doc := range docs {
		if _, err := ie.store.EmbedDocument(ie.ctx, doc, me.embedder); err != nil {
			t.Fatalf("embedding %s: %v", doc, err)
		}
	}
	return me
}

// The gate itself, at two floors: one that keeps everything the limit allows,
// and one that cuts partway through. Both matter — the limit is applied before
// the floor, so a ranker that filtered first and took eight afterwards agrees
// with production at floor 0 and disagrees the moment a floor bites.
func TestTheInMemoryRankerSelectsExactlyWhatTheSQLDoes(t *testing.T) {
	me := newMirrorEnv(t)
	question := []float32{1, 0, 0, 0}

	for _, floor := range []float64{0, 0.9, 0.99} {
		t.Run(fmt.Sprintf("floor_%.2f", floor), func(t *testing.T) {
			if err := me.setFloor(t, floor); err != nil {
				t.Fatalf("setting the corpus floor: %v", err)
			}
			_, fromSQL, err := me.store.Retrieve(me.ctx, me.corpus, "which passage is closest", me.embedder)
			if err != nil {
				t.Fatalf("Retrieve: %v", err)
			}
			inMemory := knowledge.RankInMemory(question, me.corpusIn, floor)
			assertSameSelection(t, fromSQL, inMemory)
		})
	}
}

// setFloor moves the corpus's grounding floor, which is where the SQL path reads
// it from — so both sides are asked about the same number rather than one being
// told about it.
func (me *mirrorEnv) setFloor(t *testing.T, floor float64) error {
	t.Helper()
	_, err := me.store.EditCorpus(me.ctx, me.corpus, knowledge.UpdateCorpus{MinSimilarity: &floor})
	return err
}

// assertSameSelection compares the two rankings by what a citation is made of:
// the document, where in it the passage starts, and its text. Ids are excluded
// deliberately — the in-memory ranker mints none, because there is no row for it
// to name, and a comparison that required them would be comparing storage rather
// than retrieval.
func assertSameSelection(t *testing.T, fromSQL, inMemory []knowledge.Passage) {
	t.Helper()
	if len(fromSQL) != len(inMemory) {
		t.Fatalf("the SQL path returned %d passages and the in-memory ranker %d:\n  sql: %s\n  mem: %s",
			len(fromSQL), len(inMemory), describePassages(fromSQL), describePassages(inMemory))
	}
	for i := range fromSQL {
		sql, mem := fromSQL[i], inMemory[i]
		if sql.DocumentName != mem.DocumentName || sql.Text != mem.Text || sql.StartLine != mem.StartLine {
			t.Fatalf("position %d differs:\n  sql: %s\n  mem: %s", i, describePassages(fromSQL), describePassages(inMemory))
		}
		// Cosine computed by pgvector and cosine computed in float64 agree to
		// far better than this; the tolerance is here so the gate reports a
		// DISAGREEMENT rather than a rounding difference.
		if diff := sql.Similarity - mem.Similarity; diff > 1e-6 || diff < -1e-6 {
			t.Fatalf("position %d scores %.9f in SQL and %.9f in memory", i, sql.Similarity, mem.Similarity)
		}
	}
}

func describePassages(passages []knowledge.Passage) string {
	if len(passages) == 0 {
		return "(none)"
	}
	out := ""
	for _, p := range passages {
		out += fmt.Sprintf("%s@%.4f ", p.DocumentName, p.Similarity)
	}
	return out
}
