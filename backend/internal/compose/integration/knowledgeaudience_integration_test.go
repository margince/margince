// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The audience filter of the corpus ask, over the real retrieval query.
//
// NOT RUN when it was written: it needs the integration database, which the
// author did not start. Run it with
// `make test-it DIR=backend/internal/compose/integration RUN=Audience`.

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	kitchenText = "Margince files a captured message against the account it belongs to."
	managerText = "Margince keeps a closed deal in the pipeline until a manager archives it."
)

// Two documents sit on the same axis, so ranking alone cannot tell them apart;
// only the filter can keep one out.
func audienceEnv(t *testing.T) (*ingestEnv, *steeredEmbedder, ids.UUID) {
	t.Helper()
	ie := newIngestEnv(t)
	emb := newSteeredEmbedder()
	emb.vectors[kitchenText] = []float32{1, 0, 0, 0}
	emb.vectors[managerText] = []float32{1, 0, 0, 0}
	var corpus ids.UUID
	for id, f := range map[string]struct {
		body string
		meta map[string]string
	}{
		"kitchen":   {kitchenText, map[string]string{"audience": "Bếp"}},
		"manager":   {managerText, map[string]string{"audience": "Management"}},
		"unlabeled": {"Margince never lists an unlabelled document for a filtered ask.", map[string]string{"owner": "Lan"}},
	} {
		made, _, err := ie.store.UpsertExternal(ie.ctx, gdocsCorpus, knowledge.ExternalDocument{
			ExternalID: id, Filename: id + ".md", ContentType: "text/markdown",
			Content: strings.NewReader(f.body), Meta: f.meta,
		}, ie.queue)
		if err != nil {
			t.Fatalf("file %s: %v", id, err)
		}
		doc := ids.UUID(made.Id)
		corpus = ids.UUID(made.CorpusId)
		ie.ingest(t, doc)
		if _, err := ie.store.EmbedDocument(ie.ctx, doc, emb); err != nil {
			t.Fatalf("embed %s: %v", id, err)
		}
	}
	return ie, emb, corpus
}

func TestAnAudienceFilterKeepsOtherDocumentsOutOfRetrieval(t *testing.T) {
	ie, emb, corpus := audienceEnv(t)

	// "bep" (no accent, other case) names the kitchen document only.
	state, passages, err := ie.store.RetrieveFor(ie.ctx, corpus, "anything", emb, []string{"BEP"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeAnswered)
	if len(passages) != 1 || passages[0].Text != kitchenText {
		t.Fatalf("passages = %+v, want only the kitchen document", passages)
	}

	// No filter, or an empty one, is unchanged: all three documents are in reach.
	for _, none := range [][]string{nil, {}} {
		_, all, err := ie.store.RetrieveFor(ie.ctx, corpus, "anything", emb, none)
		if err != nil || len(all) != 3 {
			t.Fatalf("unfiltered: %d passages, err %v, want 3", len(all), err)
		}
	}
}

func TestAnAudienceFilterThatMatchesNothingIsNotCovered(t *testing.T) {
	ie, emb, corpus := audienceEnv(t)
	state, passages, err := ie.store.RetrieveFor(ie.ctx, corpus, "anything", emb, []string{"Franchise owners"})
	if err != nil {
		t.Fatalf("retrieve: %v", err)
	}
	wantOutcome(t, state, crmcontracts.KnowledgeAnswerOutcomeKnowledgeAnswerOutcomeNotCovered)
	if len(passages) != 0 {
		t.Fatalf("a filter matching nothing returned %d passages", len(passages))
	}
}
