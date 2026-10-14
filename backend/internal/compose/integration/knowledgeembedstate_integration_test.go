// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A corpus says why its passages are or are not searchable. Whether a model is
// bound, it answers as System health's search-index card does.

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/knowledge"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func embedStateOf(ctx context.Context, t *testing.T, store *knowledge.Store, corpus ids.UUID) crmcontracts.KnowledgeEmbedState {
	t.Helper()
	read, err := store.ReadCorpus(ctx, corpus)
	if err != nil {
		t.Fatalf("read corpus: %v", err)
	}
	return read.Coverage.EmbedState
}

// Every state a corpus can report, walked on one corpus. No model bound, work
// owed, work done, then work owed again when the bound model moves.
func TestACorpusSaysWhyItsPassagesAreOrAreNotSearchable(t *testing.T) {
	ee := newEmbedEnv(t)
	bound := ee.store.WithEmbedIdentity(func() string { return ee.embedder.identity })

	if got := embedStateOf(ee.ctx, t, ee.store, ee.corpus); got != crmcontracts.KnowledgeEmbedStateNoEmbeddingModel {
		t.Errorf("with no embed lane bound, embed_state = %q, want no_embedding_model", got)
	}
	if got := embedStateOf(ee.ctx, t, bound, ee.corpus); got != crmcontracts.KnowledgeEmbedStateIndexing {
		t.Errorf("ingested but not yet embedded, embed_state = %q, want indexing", got)
	}
	if _, err := bound.EmbedDocument(ee.ctx, ee.doc, ee.embedder); err != nil {
		t.Fatalf("embed: %v", err)
	}
	if got := embedStateOf(ee.ctx, t, bound, ee.corpus); got != crmcontracts.KnowledgeEmbedStateComplete {
		t.Errorf("every passage embedded, embed_state = %q, want complete", got)
	}

	rebound := ee.store.WithEmbedIdentity(func() string { return "fake/other@8" })
	if got := embedStateOf(ee.ctx, t, rebound, ee.corpus); got != crmcontracts.KnowledgeEmbedStateIndexing {
		t.Errorf("vectors under a superseded model, embed_state = %q, want indexing", got)
	}

	ee.upload(t, "waiting.md", "text/markdown", prose(1))
	if got := embedStateOf(ee.ctx, t, bound, ee.corpus); got != crmcontracts.KnowledgeEmbedStateIndexing {
		t.Errorf("a document still queued, embed_state = %q, want indexing", got)
	}
}

type embedStateWire struct {
	Items []struct {
		Coverage struct {
			EmbedState string `json:"embed_state"`
		} `json:"coverage"`
	} `json:"items"`
}

type reindexAvailabilityWire struct {
	SettingsAvailability struct {
		EmbeddingReindex *bool `json:"embedding_reindex"`
	} `json:"settings_availability"`
}

// readEmbedAgreement answers what /me and the corpus list say about the embed
// lane, over the one corpus the caller created.
func readEmbedAgreement(t *testing.T, e *apptest.AppEnv) (bool, string) {
	t.Helper()
	var me reindexAvailabilityWire
	if status := e.Call(t, "GET", "/v1/me", nil, nil, &me); status != http.StatusOK {
		t.Fatalf("GET /me: status %d", status)
	}
	var corpora embedStateWire
	if status := e.Call(t, "GET", "/v1/knowledge/corpora", nil, nil, &corpora); status != http.StatusOK {
		t.Fatalf("GET /knowledge/corpora: status %d", status)
	}
	if me.SettingsAvailability.EmbeddingReindex == nil || len(corpora.Items) != 1 {
		t.Fatalf("/me availability = %v, corpora = %+v, want one of each", me.SettingsAvailability.EmbeddingReindex, corpora)
	}
	return *me.SettingsAvailability.EmbeddingReindex, corpora.Items[0].Coverage.EmbedState
}

// Wired the way cmd/api wires both, so the two pages are proven to read one
// embed lane.
func TestKnowledgeAndSystemHealthAgreeWhetherAnEmbeddingModelIsBound(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))

	t.Run("unbound", func(t *testing.T) {
		e := apptest.SetupAppWithOptions(t, compose.WithEmbedReindex(nil, nil), compose.WithCorpusAsk(nil, nil, quiet))
		e.BootstrapWorkspace(t)
		createCorpusOverHTTP(t, e)
		offered, state := readEmbedAgreement(t, e)
		if offered || state != string(crmcontracts.KnowledgeEmbedStateNoEmbeddingModel) {
			t.Errorf("unbound: reindex offered = %v, embed_state = %q, want false and no_embedding_model", offered, state)
		}
	})

	t.Run("bound", func(t *testing.T) {
		router := embedReindexRouter(t, "embed-agree")
		e := apptest.SetupAppWithOptions(t,
			compose.WithEmbedReindex(router, insertOnlyRunner(t)), compose.WithCorpusAsk(router, nil, quiet))
		e.BootstrapWorkspace(t)
		createCorpusOverHTTP(t, e)
		offered, state := readEmbedAgreement(t, e)
		if !offered || state != string(crmcontracts.KnowledgeEmbedStateComplete) {
			t.Errorf("bound: reindex offered = %v, embed_state = %q, want true and an empty corpus complete", offered, state)
		}
	})
}

// The routing watcher rebinds a running role, so a lane bound or unbound after
// boot moves both pages together.
func TestKnowledgeAndSystemHealthAgreeAcrossARebind(t *testing.T) {
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	modelPath, err := compose.NewLocalModelPath(ai.FakeRoutingConfig(), ai.WithoutResultCache())
	if err != nil {
		t.Fatalf("NewLocalModelPath: %v", err)
	}
	router := modelPath.Router()
	e := apptest.SetupAppWithOptions(t,
		compose.WithEmbedReindex(router, insertOnlyRunner(t)), compose.WithCorpusAsk(router, nil, quiet))
	e.BootstrapWorkspace(t)
	createCorpusOverHTTP(t, e)

	steps := []struct {
		name    string
		routing ai.RoutingConfig
		offered bool
		state   crmcontracts.KnowledgeEmbedState
	}{
		{"booted unbound", ai.FakeRoutingConfig(), false, crmcontracts.KnowledgeEmbedStateNoEmbeddingModel},
		{"bound after boot", boundEmbedRouting("embed-rebound"), true, crmcontracts.KnowledgeEmbedStateComplete},
		{"unbound again", ai.FakeRoutingConfig(), false, crmcontracts.KnowledgeEmbedStateNoEmbeddingModel},
	}
	for _, step := range steps {
		if err := router.Rebind(step.routing); err != nil {
			t.Fatalf("%s: rebind: %v", step.name, err)
		}
		offered, state := readEmbedAgreement(t, e)
		if offered != step.offered || state != string(step.state) {
			t.Errorf("%s: reindex offered = %v, embed_state = %q, want %v and %q",
				step.name, offered, state, step.offered, step.state)
		}
		status := e.Call(t, "GET", "/v1/embeddings/reindex/status", nil, nil, nil)
		if served := status != http.StatusNotImplemented; served != step.offered {
			t.Errorf("%s: reindex status answered %d while /me offers the surface = %v", step.name, status, step.offered)
		}
	}
}
