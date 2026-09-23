// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// What the DB-less corpus probe chunks, what it pays for twice, and what it
// refuses — without a model and without a database.
//
// The one thing NOT covered here is whether the in-process ranking agrees with
// the SQL. That needs Postgres, and it is the mirror gate in the integration
// lane (TestTheInMemoryRankerSelectsExactlyWhatTheSQLDoes).

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// countingEmbedder answers a deterministic vector per input and records how many
// inputs it was asked about, which is what makes the cache's claim checkable: a
// cache that was not consulted looks exactly like one that was, unless somebody
// counts.
type countingEmbedder struct {
	dims   int
	inputs int
}

func (e *countingEmbedder) EmbedIdentity() (string, int) { return "fake/count@4", e.dims }

func (e *countingEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	e.inputs += len(req.Inputs)
	out := make([][]float32, len(req.Inputs))
	for i, in := range req.Inputs {
		out[i] = axisVector(in, e.dims)
	}
	return model.Embeddings{Vectors: out, Dims: e.dims}, nil
}

// axisVector places a string somewhere reproducible in the unit cube. It is not
// a model and does not pretend to be: what the tests below need is that the same
// text always lands in the same place, so a ranking is an assertion rather than
// a coin toss.
func axisVector(text string, dims int) []float32 {
	vec := make([]float32, dims)
	for i, r := range text {
		vec[i%dims] += float32(r%7) + 1
	}
	if vec[0] == 0 {
		vec[0] = 1
	}
	return vec
}

// The corpus is the handbook this binary ships, cut by the production chunker —
// not by anything written here. The width and the start line are what a fixture
// typed by hand gets wrong, so both are asserted.
func TestTheCorpusIsTheShippedHandbookCutByTheProductionChunker(t *testing.T) {
	corpus, err := handbookChunks("records.md")
	if err != nil {
		t.Fatalf("chunking records.md: %v", err)
	}
	if len(corpus) < 2 {
		t.Fatalf("records.md produced %d chunks, want several — a one-chunk page is not the shape retrieval ranks", len(corpus))
	}
	for i, p := range corpus {
		if p.DocumentName != "records.md" {
			t.Fatalf("chunk %d came from %q", i, p.DocumentName)
		}
		if p.Chunk.StartLine < 1 {
			t.Errorf("chunk %d has no start line, so a citation from it could not be followed", i)
		}
		if len([]rune(p.Chunk.Text)) > 800 {
			t.Errorf("chunk %d is %d characters, past the chunker's own ceiling", i, len([]rune(p.Chunk.Text)))
		}
	}
	whole, err := handbookChunks("")
	if err != nil {
		t.Fatalf("chunking the whole handbook: %v", err)
	}
	if len(whole) <= len(corpus) {
		t.Errorf("the whole handbook produced %d chunks and one page produced %d", len(whole), len(corpus))
	}
}

func TestAPageThisBuildDoesNotShipIsRefusedByName(t *testing.T) {
	_, err := handbookChunks("not-a-page.md")
	if err == nil {
		t.Fatal("a page that does not exist was accepted, so the run would have ranked nothing and said 'not covered'")
	}
	if !strings.Contains(err.Error(), "records.md") {
		t.Errorf("the refusal does not name the pages this build ships: %v", err)
	}
}

// The cache's whole claim: identical prose costs nothing the second time. It is
// asserted by counting inputs, because a cache that is never read is invisible
// in the output.
func TestUnchangedProseIsEmbeddedOnlyOnce(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	corpus, err := handbookChunks("records.md")
	if err != nil {
		t.Fatalf("chunking: %v", err)
	}
	first := &countingEmbedder{dims: 4}
	if _, err := embedChunks(ctx, corpus, first, newVectorCache(dir, "fake/count@4"), 4); err != nil {
		t.Fatalf("first embed: %v", err)
	}
	if first.inputs != len(corpus) {
		t.Fatalf("the first run embedded %d of %d chunks", first.inputs, len(corpus))
	}

	again, err := handbookChunks("records.md")
	if err != nil {
		t.Fatalf("chunking: %v", err)
	}
	second := &countingEmbedder{dims: 4}
	embedded, err := embedChunks(ctx, again, second, newVectorCache(dir, "fake/count@4"), 4)
	if err != nil {
		t.Fatalf("second embed: %v", err)
	}
	if second.inputs != 0 {
		t.Errorf("the second run re-embedded %d chunks; the content hash exists so it would not", second.inputs)
	}
	for i, p := range embedded {
		if len(p.Vector) != 4 {
			t.Fatalf("chunk %d came back from the cache with a %d-wide vector", i, len(p.Vector))
		}
	}
}

// A cache written under one binding must not be served to another: the two
// vectors live in different spaces, and ranking across them is meaningless.
func TestAVectorCacheIsNotSharedBetweenBindings(t *testing.T) {
	dir := t.TempDir()
	mistral := newVectorCache(dir, "mistral/mistral-embed@1024")
	gemini := newVectorCache(dir, "gemini/gemini-embedding-001@1024")
	if mistral.path == gemini.path {
		t.Fatalf("both bindings cache to %s, so one would be served the other's vectors", mistral.path)
	}
	if filepath.Dir(mistral.path) != filepath.Join(dir, "vectors") {
		t.Errorf("the cache landed at %s, outside the work directory it must stay inside", mistral.path)
	}
}

// The end of the loop: the ranking becomes the fixture the corpus_ask site
// decodes, which is what makes `retrieve` and `run` one workflow rather than two
// commands that nearly fit together.
func TestTheRankingRendersTheFixtureTheSiteDecodes(t *testing.T) {
	ctx := context.Background()
	corpus, err := handbookChunks("records.md")
	if err != nil {
		t.Fatalf("chunking: %v", err)
	}
	embedder := &countingEmbedder{dims: 4}
	embedded, err := embedChunks(ctx, corpus, embedder, newVectorCache(t.TempDir(), "fake/count@4"), 4)
	if err != nil {
		t.Fatalf("embedding: %v", err)
	}
	question, err := embedQuestion(ctx, "how can I create a project", embedder, 4)
	if err != nil {
		t.Fatalf("embedding the question: %v", err)
	}
	// A floor of zero, because what is under test is the rendering and not where
	// this stand-in embedder happens to place things.
	passages := knowledge.RankInMemory(question, embedded, 0)
	if len(passages) != CorpusProbeLimit {
		t.Fatalf("ranked %d passages, want the site's own limit of %d", len(passages), CorpusProbeLimit)
	}
	doc, err := CorpusAskFixtureFrom("how can I create a project", passages)
	if err != nil {
		t.Fatalf("rendering the fixture: %v", err)
	}
	var fixture struct {
		Question string `json:"question"`
		Passages []struct {
			Label    string `json:"label"`
			Document string `json:"document"`
			Text     string `json:"text"`
		} `json:"passages"`
	}
	if err := json.Unmarshal(doc, &fixture); err != nil {
		t.Fatalf("the fixture is not the shape the site decodes: %v", err)
	}
	if fixture.Question != "how can I create a project" {
		t.Errorf("fixture question = %q", fixture.Question)
	}
	if len(fixture.Passages) != len(passages) {
		t.Fatalf("the fixture carries %d passages, the ranking had %d", len(fixture.Passages), len(passages))
	}
	for i, p := range fixture.Passages {
		if p.Document != passages[i].DocumentName || p.Text != passages[i].Text {
			t.Errorf("passage %d is not the one ranked at that position", i)
		}
		if p.Label == "" {
			t.Errorf("passage %d has no label, and an expectation names passages by label", i)
		}
	}
}
