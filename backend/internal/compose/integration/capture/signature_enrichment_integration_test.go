// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// The per-mailbox answer to the nightly signature pass: setting it, reading it
// back through the list surface, and the tri-state that makes it more than a
// boolean.
//
// Against a real database because the third state IS a null, and a null that
// round-tripped as `false` would silently pin every mailbox to whatever the
// tenant default said on the day it was written — the exact defect the nullable
// column exists to avoid, and one no fake store can show.
//
// The connection is made through registry.Connect rather than an INSERT, so the
// row under test is the row the product writes.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	capturemod "github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// connectedMailbox builds a registry with one connected mailbox for e.Rep1,
// and hands back the registry plus the context that owns it.
func connectedMailbox(t *testing.T, e *integration.SearchEnv) (*capturemod.Registry, context.Context) {
	t.Helper()
	registry := capturemod.NewRegistry(e.DB(), capturemod.NewSink(e.DB()), fakeAuthority{}, newTestKeyvault(t, e))
	registry.Register(&grantingFake{granted: []string{"Mail.Read"}})
	owner := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})
	if _, err := registry.Connect(owner, "graph", connector.Auth("token")); err != nil {
		t.Fatalf("connecting the mailbox: %v", err)
	}
	return registry, owner
}

func listedAnswer(ctx context.Context, t *testing.T, registry *capturemod.Registry) *bool {
	t.Helper()
	views, err := registry.Connections(ctx)
	if err != nil {
		t.Fatalf("listing connections: %v", err)
	}
	for _, v := range views {
		if v.Provider == "graph" {
			return v.SignatureEnrichEnabled
		}
	}
	t.Fatal("the connected mailbox is not in the list")
	return nil
}

func TestAMailboxStartsWithNoAnswerOfItsOwn(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)

	// A freshly connected mailbox follows the tenant default, and says so by
	// holding no answer rather than by being written to the default's current
	// value — which would freeze it there.
	if got := listedAnswer(owner, t, registry); got != nil {
		t.Errorf("a new mailbox's answer = %v, want nil (following the default)", *got)
	}
}

func TestAMailboxKeepsItsOwnSignatureAnswer(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)

	off := false
	view, err := registry.SetSignatureEnrichment(owner, "graph", &off)
	if err != nil {
		t.Fatalf("switching the mailbox off: %v", err)
	}
	if view.SignatureEnrichEnabled == nil || *view.SignatureEnrichEnabled {
		t.Fatalf("the write answered %v, want false", view.SignatureEnrichEnabled)
	}
	// Read back through the list surface, which is what the settings screen
	// draws: a write that answered correctly and stored something else would
	// show the reader their own change and lose it on reload.
	got := listedAnswer(owner, t, registry)
	if got == nil || *got {
		t.Errorf("the listed answer = %v, want false", got)
	}
}

func TestAMailboxCanHandTheQuestionBackToTheTenantDefault(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)

	on := true
	if _, err := registry.SetSignatureEnrichment(owner, "graph", &on); err != nil {
		t.Fatalf("switching the mailbox on: %v", err)
	}
	if _, err := registry.SetSignatureEnrichment(owner, "graph", nil); err != nil {
		t.Fatalf("handing the question back: %v", err)
	}
	if got := listedAnswer(owner, t, registry); got != nil {
		t.Errorf("the answer after handing back = %v, want nil", *got)
	}
}

func TestSettingSignatureEnrichmentAuditsTheChange(t *testing.T) {
	e := integration.SetupSearch(t)
	registry, owner := connectedMailbox(t, e)

	off := false
	if _, err := registry.SetSignatureEnrichment(owner, "graph", &off); err != nil {
		t.Fatalf("switching the mailbox off: %v", err)
	}

	var before, after *bool
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(), `
			SELECT (before->>'signature_enrich_enabled')::boolean,
			       (after->>'signature_enrich_enabled')::boolean
			  FROM audit_log
			 WHERE entity_type = 'capture_settings' AND after->>'provider' = 'graph'
			 ORDER BY occurred_at DESC
			 LIMIT 1`).Scan(&before, &after)
	}); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	// The before-image is the null the mailbox started at, which is what makes
	// the trail answer "what did it say before" rather than only "what now".
	if before != nil {
		t.Errorf("the before-image = %v, want nil", *before)
	}
	if after == nil || *after {
		t.Errorf("the after-image = %v, want false", after)
	}
}

func TestSettingSignatureEnrichmentRefusesAMailboxTheCallerHasNot(t *testing.T) {
	e := integration.SetupSearch(t)
	registry := capturemod.NewRegistry(e.DB(), capturemod.NewSink(e.DB()), fakeAuthority{}, newTestKeyvault(t, e))
	registry.Register(&grantingFake{granted: []string{"Mail.Read"}})
	caller := humanWithScopes(e, e.Rep1, []principal.Scope{principal.ScopeRead})

	// Nothing connected: the answer is that the mailbox is not there, rather
	// than a write that silently touches no row and reports success.
	off := false
	if _, err := registry.SetSignatureEnrichment(caller, "graph", &off); err == nil {
		t.Fatal("setting an answer on an unconnected mailbox succeeded")
	}
}
