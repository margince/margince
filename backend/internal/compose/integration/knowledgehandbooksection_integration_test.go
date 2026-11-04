// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The shipped handbook is filed and embedded the way a boot does it. A time zone
// question then gets the section it quotes and that section's settings page.

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/modules/knowledge/handbook"
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
	embedder := sectionEmbedder{near: timeZoneHeading, question: question}
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
	// Chunks overlap, so the section can sit in two neighbouring passages; how
	// many depends on where the chunker cut, which any edit above it moves.
	quote, opens := timeZoneSection(t)
	quoted := -1
	for i, passage := range passages {
		if !strings.Contains(passage.Text, embedder.near) {
			t.Fatalf("retrieved a passage without the time zone section: %q", passage.Text)
		}
		if quoted < 0 && strings.Contains(passage.Text, quote) {
			quoted = i
		}
	}
	if quoted < 0 {
		t.Fatalf("none of the %d passages holds the time zone section's text", len(passages))
	}
	quoteJSON, err := json.Marshal(quote)
	if err != nil {
		t.Fatalf("encode quote: %v", err)
	}
	lane := writtenLane{reply: `{"coverage":"answers","summary":"Pick it in your own settings. [1]","claims":[` +
		`{"text":"Pick your time zone in your own settings.","id":"` + passages[quoted].ChunkID.String() +
		`","quote":` + string(quoteJSON) + `}]}`}
	answer := compose.AnswerCorpus(ie.ctx, lane, state, question, passages,
		string(textlang.English), quietAskLog())
	if answer.Claims == nil || len(*answer.Claims) != 1 {
		t.Fatalf("outcome %s carried claims %v, want the one written claim", answer.Outcome, answer.Claims)
	}
	claim := (*answer.Claims)[0]
	if claim.Section == nil || claim.Section.Title != "How do I change my time zone?" {
		t.Fatalf("the claim names section %+v, want the time zone section", claim.Section)
	}
	if claim.Opens == nil || claim.Opens.SettingsPage != opens {
		t.Fatalf("the time zone section opens %+v, want settings page %s", claim.Opens, opens)
	}
}

const (
	ownSettingsPage = "your-own-settings.md"
	timeZoneHeading = "### How do I change my time zone?"
)

// timeZoneSection reads the shipped time zone section: its first line a reader
// sees, and the settings page it names.
func timeZoneSection(t *testing.T) (quote, opens string) {
	t.Helper()
	content, ok := handbook.Open(ownSettingsPage)
	if !ok {
		t.Fatalf("this build ships no %s", ownSettingsPage)
	}
	lines := strings.Split(string(content), "\n")
	for i, line := range lines {
		if line != timeZoneHeading {
			continue
		}
		section, ok := handbook.SectionFor(ownSettingsPage, i+1, i+1)
		if !ok || section.Opens == "" {
			t.Fatalf("the shipped time zone section names no settings page: %+v", section)
		}
		for _, under := range lines[i+1:] {
			if strings.HasPrefix(under, "#") {
				break
			}
			if under != "" && !strings.HasPrefix(under, "<!--") {
				return under, section.Opens
			}
		}
	}
	t.Fatalf("no %q with text under it in %s", timeZoneHeading, ownSettingsPage)
	return "", ""
}

// writtenLane is the chat lane, answering with one fixed reply.
type writtenLane struct{ reply string }

func (l writtenLane) Complete(context.Context, model.Request) (model.Response, error) {
	return model.Response{Text: l.reply}, nil
}
