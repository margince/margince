// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The deterministic ask suite: the three refusals a corpus can honestly give,
// and the one case that reaches passages. No model is involved in any of them,
// which is the property under test — a refusal that depended on a model call
// could not say why it refused.
//
// The three are never interchangeable:
//   not_ready              is about the CORPUS
//   retrieval_unavailable  is about the INSTALLATION
//   not_covered            is about the QUESTION
//
// Answering "your documents do not cover this" for a corpus that is merely
// half-ingested is an affirmative false claim about documents that do.

import (
	"context"
	"strings"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/knowledge"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/ports/model"
)

// steeredEmbedder answers a vector chosen by the caller, so a test can place a
// question at a KNOWN cosine from a passage rather than hoping real prose lands
// where it wants. The floor is the subject of several cases below and a fixture
// that could not aim would be testing the fixture.
type steeredEmbedder struct {
	identity string
	dims     int
	// vectors maps an exact input string to the vector to answer with; any
	// input not listed gets `fallback`.
	vectors  map[string][]float32
	fallback []float32
	calls    int
}

func newSteeredEmbedder() *steeredEmbedder {
	return &steeredEmbedder{
		identity: "fake/steer@4",
		dims:     4,
		vectors:  map[string][]float32{},
		fallback: []float32{1, 0, 0, 0},
	}
}

func (e *steeredEmbedder) EmbedIdentity() (string, int) { return e.identity, e.dims }

func (e *steeredEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	e.calls++
	out := make([][]float32, len(req.Inputs))
	for i, in := range req.Inputs {
		if v, ok := e.vectors[in]; ok {
			out[i] = v
			continue
		}
		out[i] = e.fallback
	}
	return model.Embeddings{Vectors: out, Dims: e.dims}, nil
}

// askEnv is a corpus with one ingested, embedded document.
type askEnv struct {
	*ingestEnv
	doc      ids.UUID
	embedder *steeredEmbedder
}

// onePassage is short enough that the chunker yields exactly one span, so a
// test can point the embedder at it by its exact text.
const onePassage = "Margince files a captured message against the account it belongs to."

func newAskEnv(t *testing.T) *askEnv {
	t.Helper()
	ie := newIngestEnv(t)
	ae := &askEnv{ingestEnv: ie, embedder: newSteeredEmbedder()}
	ae.doc = ie.upload(t, "operating.md", "text/markdown", onePassage)
	ie.ingest(t, ae.doc)
	// The stored passage sits on one axis; a question's closeness to it is then
	// whatever the test points the question vector at.
	ae.embedder.vectors[onePassage] = []float32{1, 0, 0, 0}
	if _, err := ie.store.EmbedDocument(ie.ctx, ae.doc, ae.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	return ae
}

func (ae *askEnv) ask(t *testing.T, question string) (knowledge.Readiness, []knowledge.Passage) {
	t.Helper()
	state, passages, err := ae.store.Retrieve(ae.ctx, ae.corpus, question, ae.embedder)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	return state, passages
}

func wantOutcome(t *testing.T, got knowledge.Readiness, want crmcontracts.KnowledgeAnswerOutcome) {
	t.Helper()
	if got.Outcome != want {
		t.Fatalf("outcome = %q, want %q", got.Outcome, want)
	}
}

// A corpus mid-ingest retrieves nothing. Answering "your documents do not cover
// this" would be an affirmative false claim about documents that do.
func TestACorpusWithADocumentStillIngestingIsNotReady(t *testing.T) {
	ae := newAskEnv(t)
	// A second document uploaded and left queued, which is exactly the state
	// between the upload's 202 and the worker picking it up.
	ae.upload(t, "second.md", "text/markdown", prose(1))

	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotReady)
	if len(passages) != 0 {
		t.Fatalf("a not_ready corpus returned %d passages", len(passages))
	}
}

func TestACorpusWithNoDocumentsAtAllIsNotReady(t *testing.T) {
	ie := newIngestEnv(t)
	state, passages, err := ie.store.Retrieve(ie.ctx, ie.corpus, "anything", newSteeredEmbedder())
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotReady)
	if len(passages) != 0 {
		t.Fatalf("an empty corpus returned %d passages", len(passages))
	}
}

// A failed document leaves the denominator, or one bad file makes the whole
// corpus unanswerable forever. Its own row already says it failed and why.
func TestAFailedDocumentDoesNotHoldTheCorpusNotReady(t *testing.T) {
	ae := newAskEnv(t)
	bad := ae.upload(t, "broken.md", "text/markdown", prose(1))
	if err := ae.store.FailIngest(ae.ctx, bad, "the stored file could not be read"); err != nil {
		t.Fatalf("fail: %v", err)
	}

	ae.embedder.vectors["how is a message filed"] = []float32{1, 0, 0, 0}
	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeAnswered)
	if len(passages) == 0 {
		t.Fatal("a failed sibling document made the corpus unanswerable")
	}
}

// Vectors under a superseded identity retrieve nothing — which is not_ready,
// never not_covered. The corpus IS holding the prose; nothing about the
// question is wrong.
func TestVectorsUnderASupersededIdentityReadAsNotReady(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.identity = "fake/other@4"

	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotReady)
	if len(passages) != 0 {
		t.Fatalf("a superseded-binding corpus returned %d passages", len(passages))
	}
}

func TestNoEmbedLaneBoundIsRetrievalUnavailable(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.identity = ""
	// Counted from here, not from zero: the fixture embedded the document to
	// get the corpus into a state worth asking.
	callsBefore := ae.embedder.calls

	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeRetrievalUnavailable)
	if len(passages) != 0 {
		t.Fatalf("an unbound installation returned %d passages", len(passages))
	}
	// Nothing was searched, and nothing was asked of a lane that is not there —
	// not even the question's own embedding.
	if ae.embedder.calls != callsBefore {
		t.Fatalf("%d model call(s) with no lane bound", ae.embedder.calls-callsBefore)
	}
}

// Below the floor is not_covered, and it is settled without a model. A passage
// under the floor must never reach one: a claim citing it would pass the quote
// check and re-open exactly the hole the floor exists to close.
func TestAQuestionBelowTheFloorIsNotCoveredWithoutAModelCall(t *testing.T) {
	ae := newAskEnv(t)
	// Orthogonal to the stored passage: cosine 0, far below the 0.35 default.
	ae.embedder.vectors["what is the capital of France"] = []float32{0, 1, 0, 0}
	callsBefore := ae.embedder.calls

	state, passages := ae.ask(t, "what is the capital of France")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotCovered)
	if len(passages) != 0 {
		t.Fatalf("a sub-floor question returned %d passages", len(passages))
	}
	// Exactly one call, and it is the QUESTION's own embedding — the refusal
	// itself cost no chat lane.
	if ae.embedder.calls != callsBefore+1 {
		t.Fatalf("the refusal made %d calls, want the question's embed alone", ae.embedder.calls-callsBefore)
	}
}

func TestAQuestionJustAboveTheFloorRetrievesPassages(t *testing.T) {
	ae := newAskEnv(t)
	// cos = 0.6 / (1 * sqrt(0.6² + 0.8²)) = 0.6, comfortably over the 0.35
	// default and chosen rather than hoped for.
	ae.embedder.vectors["how is a message filed"] = []float32{0.6, 0.8, 0, 0}

	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeAnswered)
	if len(passages) != 1 {
		t.Fatalf("retrieved %d passages, want 1", len(passages))
	}
	p := passages[0]
	if p.Text != onePassage {
		t.Fatalf("retrieved %q, want the stored passage", p.Text)
	}
	// The citation has to point at something a reader can open.
	if p.DocumentName != "operating.md" || p.DocumentID.String() != ae.doc.String() {
		t.Fatalf("the passage cites %s / %s, want operating.md / %s", p.DocumentName, p.DocumentID, ae.doc)
	}
	if p.Similarity < 0.35 {
		t.Fatalf("similarity %v is below the floor it was supposed to clear", p.Similarity)
	}
}

// The floor is the CORPUS's, not a constant: raising it turns the same question
// into a refusal, which is what the setting is for.
func TestRaisingTheFloorTurnsTheSameQuestionIntoARefusal(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["how is a message filed"] = []float32{0.6, 0.8, 0, 0}
	state, _ := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeAnswered)

	floor := 0.9
	if _, err := ae.store.EditCorpus(ae.ctx, ae.corpus, knowledge.UpdateCorpus{MinSimilarity: &floor}); err != nil {
		t.Fatalf("raise the floor: %v", err)
	}
	state, passages := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotCovered)
	if len(passages) != 0 {
		t.Fatalf("a raised floor still returned %d passages", len(passages))
	}
}

// An archived document's passages are never retrieved. They would otherwise be
// cited by name out of a file the screen no longer lists.
func TestAnArchivedDocumentsChunksAreNeverRetrieved(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["how is a message filed"] = []float32{1, 0, 0, 0}
	if _, passages := ae.ask(t, "how is a message filed"); len(passages) == 0 {
		t.Fatal("nothing was retrievable before the archive, so this proves nothing")
	}
	// The document's chunks are stamped exactly as the archive path stamps them.
	ae.env.WsExec(t, `UPDATE knowledge_chunk SET archived_at = now() WHERE document_id = $1`, ae.doc)
	ae.env.WsExec(t, `UPDATE knowledge_document SET archived_at = now() WHERE id = $1`, ae.doc)

	state, passages := ae.ask(t, "how is a message filed")
	if len(passages) != 0 {
		t.Fatalf("an archived document's passages were retrieved: %d", len(passages))
	}
	// And the corpus says it holds nothing rather than blaming the question.
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotReady)
}

// A corpus being re-embedded says so. not_ready without the distinction invites
// the reader to go and finish uploading something; there is nothing for them to
// do but wait.
func TestACorpusBeingReindexedIsNotReady(t *testing.T) {
	ae := newAskEnv(t)
	ae.env.WsExec(t, `UPDATE knowledge_corpus SET reindexing = true WHERE id = $1`, ae.corpus)

	state, _ := ae.ask(t, "how is a message filed")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotReady)
	if state.Corpus.TopicStatement == "" {
		t.Fatal("the refusal carries no topic statement to quote back")
	}
}

// An empty question is not_covered, and costs nothing: there is no vector to
// rank and nothing honest to say except that the corpus does not cover it.
func TestAnEmptyQuestionIsNotCoveredWithoutEmbeddingAnything(t *testing.T) {
	ae := newAskEnv(t)
	callsBefore := ae.embedder.calls

	state, passages := ae.ask(t, "   \n  ")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotCovered)
	if len(passages) != 0 {
		t.Fatalf("an empty question returned %d passages", len(passages))
	}
	if ae.embedder.calls != callsBefore {
		t.Fatalf("an empty question cost %d model call(s)", ae.embedder.calls-callsBefore)
	}
}

// A question the lane can only answer with a zero vector is not_covered rather
// than a crash: every cosine against zero is NaN and `ORDER BY sim DESC` sorts
// NaN FIRST, so ranking it would put arbitrary passages at the top.
func TestAZeroQuestionVectorIsNotCoveredRatherThanRanked(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["???"] = []float32{0, 0, 0, 0}

	state, passages := ae.ask(t, "???")
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeNotCovered)
	if len(passages) != 0 {
		t.Fatalf("a zero question vector ranked %d passages", len(passages))
	}
}

// Every outcome carries the corpus's name and topic statement, because the
// refusal quotes them back — a reader at their least patient moment needs to be
// told WHAT was searched.
func TestEveryOutcomeNamesWhatWasSearched(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["orthogonal"] = []float32{0, 1, 0, 0}
	ae.embedder.vectors["close"] = []float32{1, 0, 0, 0}

	for _, question := range []string{"orthogonal", "close"} {
		state, _ := ae.ask(t, question)
		if state.Corpus.Name != "How-to" {
			t.Fatalf("%q: corpus name is %q", question, state.Corpus.Name)
		}
		if !strings.Contains(state.Corpus.TopicStatement, "operated") {
			t.Fatalf("%q: topic statement is %q", question, state.Corpus.TopicStatement)
		}
		if state.Corpus.Id.String() != ae.corpus.String() {
			t.Fatalf("%q: corpus id is %s", question, state.Corpus.Id)
		}
	}
}

// A rep may ask. Read is the ask, and every role that reads records holds it —
// otherwise the help bot is an admin tool.
func TestARepMayAskACorpus(t *testing.T) {
	ae := newAskEnv(t)
	ae.embedder.vectors["how is a message filed"] = []float32{1, 0, 0, 0}
	rep := ae.env.As(ae.env.Rep1, nil, corpusRepPerms)

	state, passages, err := ae.store.Retrieve(rep, ae.corpus, "how is a message filed", ae.embedder)
	if err != nil {
		t.Fatalf("a rep must be able to ask a corpus: %v", err)
	}
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeAnswered)
	if len(passages) == 0 {
		t.Fatal("a rep's ask retrieved nothing")
	}
}
