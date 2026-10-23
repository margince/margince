// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The shipped handbook is filed and embedded the way a boot does it. A time zone
// question then gets the section it quotes and that section's settings page.

import (
	"context"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

// sectionEmbedder places the question and every passage holding `near` on one
// axis and every other passage on another, so the question lands on that
// section alone.
type sectionEmbedder struct{ near, question string }

func (sectionEmbedder) EmbedIdentity() (string, int) { return "gemini/gemini-embedding-001@4", 4 }

func (s sectionEmbedder) Embed(_ context.Context, req model.EmbedRequest) (model.Embeddings, error) {
	out := make([][]float32, len(req.Inputs))
	for i, in := range req.Inputs {
		out[i] = []float32{0, 1, 0, 0}
		if in == s.question || strings.Contains(in, s.near) {
			out[i] = []float32{1, 0, 0, 0}
		}
	}
	return model.Embeddings{Vectors: out, Dims: 4}, nil
}

func TestTheShippedHandbookAnswersATimeZoneQuestionWithItsSettingsPage(t *testing.T) {
	e := Setup(t)
	store := knowledge.NewStore(e.DB())
	var q recordingQueue
	if _, err := store.ReconcileHandbook(handbookBootCtx(e.WS), noSerialize, q.queue()); err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	ie := &ingestEnv{env: e, store: store, ctx: e.As(e.Rep1, nil, corpusAdminPerms)}
	ie.corpus = handbookCorpusID(ie.ctx, t, e)
	const question = "how do I change my time zone"
	embedder := sectionEmbedder{near: "### How do I change my time zone?", question: question}
	for _, doc := range q.queued {
		ie.ingest(t, doc)
		if _, err := store.EmbedDocument(ie.ctx, doc, embedder); err != nil {
			t.Fatalf("embed: %v", err)
		}
	}

	state, passages, err := store.Retrieve(ie.ctx, ie.corpus, question, embedder)
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	if !state.Corpus.Shipped {
		t.Fatal("the handbook set's answer does not say it is the shipped set")
	}
	if len(passages) != 1 {
		t.Fatalf("retrieved %d passages, want the one holding the time zone section", len(passages))
	}
	lane := writtenLane{reply: `{"coverage":"answers","summary":"Pick it on the Meetings page. [1]","claims":[` +
		`{"text":"Pick your time zone on the Meetings page.","id":"` + passages[0].ChunkID.String() +
		`","quote":"choose a place in the **Timezone** list"}]}`}
	answer := compose.AnswerCorpus(ie.ctx, lane, state, question, passages,
		string(textlang.English), quietAskLog())
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("outcome %s carried claims %v, want the one written claim", answer.Outcome, answer.Claims)
	}
	claim := (*answer.Claims)[0]
	if claim.Section == nil || claim.Section.Title != "How do I change my time zone?" {
		t.Fatalf("the claim names section %+v, want the time zone section", claim.Section)
	}
	if claim.Opens == nil || claim.Opens.SettingsPage != "meetings" {
		t.Fatalf("the time zone section opens %+v, want settings page meetings", claim.Opens)
	}
}

// writtenLane is the chat lane, answering with one fixed reply.
type writtenLane struct{ reply string }

func (l writtenLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}
