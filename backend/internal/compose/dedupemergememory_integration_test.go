// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a rep's "no" means to a captured merge proposal.
//
// A connector re-syncs the same upstream record every cycle, so it hits the same
// collision every cycle. The pending check the stager had could absorb the
// repeat only while a proposal was still waiting — and a rejection is exactly
// what stops it waiting. So a rep who said "these are not the same person" was
// asked again on the next sync, and every sync after, until they gave in.
//
// Giving in here is destructive: a merge is the one action that removes the
// distinction between two records.
//
// SQL throughout, and the sink's own path: what the connector staged, and what
// the memory matched.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// mergeEnv drives the real capture sink over one workspace, with the approvals
// service the composition root gives it.
type mergeEnv struct {
	*integration.Env
	sink *capture.Sink
	svc  *approvals.Service
}

func setupMerge(t *testing.T) *mergeEnv {
	t.Helper()
	e := integration.Setup(t)
	svc := approvals.NewService(e.DB())
	return &mergeEnv{Env: e, sink: capture.NewSink(e.DB()).WithStager(mergeStager{svc: svc}), svc: svc}
}

// capturedLead runs one upstream record through the sink, exactly as a connector
// sync does.
func (e *mergeEnv) capturedLead(t *testing.T, source, sourceID, name, email string) ids.UUID {
	t.Helper()
	ref, err := e.sink.Upsert(connectorCtx(e.Env), connector.NormalizedRecord{
		EntityType: "lead",
		NaturalKey: connector.NaturalKey{SourceSystem: source, SourceID: sourceID},
		Fields:     capture.LeadFields{FullName: name, Email: email},
		Source:     source + ":" + sourceID, CapturedBy: "connector:test",
	})
	if err != nil {
		t.Fatalf("capturing %s/%s: %v", source, sourceID, err)
	}
	return ref.ID
}

// pendingMerges counts the merge proposals standing against one lead.
func (e *mergeEnv) pendingMerges(t *testing.T, leadID ids.UUID) int {
	t.Helper()
	var n int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE kind = 'merge_records'
			   AND target_entity_id = $1 AND status = 'pending'`, leadID).Scan(&n)
	}); err != nil {
		t.Fatalf("counting merge proposals: %v", err)
	}
	return n
}

// rejectMerge turns down the merge proposal standing against one lead.
func (e *mergeEnv) rejectMerge(t *testing.T, leadID ids.UUID) {
	t.Helper()
	var approvalID ids.ApprovalID
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT id FROM approval WHERE kind = 'merge_records'
			   AND target_entity_id = $1 AND status = 'pending'`, leadID).Scan(&approvalID)
	}); err != nil {
		t.Fatalf("no staged merge to reject: %v", err)
	}
	human := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	if _, err := e.svc.Decide(human, approvalID, false, nil); err != nil {
		t.Fatalf("rejecting the merge: %v", err)
	}
}

// A refused merge is not proposed again on the next sync.
//
// The plain case: the connector re-sends exactly what it sent before. This one
// is answered by the declined check itself, whose fallback discriminator is the
// payload's diff hash; the identity earns its place in the test below, where the
// payload has moved.
func TestARejectedMergeIsNotProposedAgainOnTheNextSync(t *testing.T) {
	e := setupMerge(t)
	incumbent := e.capturedLead(t, "apollo", "a-1", "Dana Dup", "dana@example.test")
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	if got := e.pendingMerges(t, incumbent); got != 1 {
		t.Fatalf("the first collision staged %d proposals, want 1", got)
	}
	e.rejectMerge(t, incumbent)

	// The connector syncs again, and its cursor was stale, so it re-sends the
	// record it already sent.
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	if got := e.pendingMerges(t, incumbent); got != 0 {
		t.Errorf("after a rejection the next sync staged %d merge proposals, want 0 — "+
			"the rep is asked to merge two records they said were different", got)
	}
}

// A re-sync whose payload has changed is still the same question.
//
// This is the case that needs the IDENTITY, and it is the only one here that
// does: StageUnlessDeclined falls back to the whole-payload diff hash when no
// identity is declared, which already answers a byte-identical re-sync. What it
// cannot answer is an upstream record that gains a title or has its name
// corrected between one sync and the next — the same two records and the same
// question, with a different payload. Since provider fields drift constantly,
// this is the common case rather than the exotic one.
func TestARejectedMergeStaysRefusedWhenTheCapturedFieldsChange(t *testing.T) {
	e := setupMerge(t)
	incumbent := e.capturedLead(t, "apollo", "a-1", "Dana Dup", "dana@example.test")
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	e.rejectMerge(t, incumbent)

	// Upstream fills in a fuller name for the same person at the same address.
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate-Smith", "dana@example.test")
	if got := e.pendingMerges(t, incumbent); got != 0 {
		t.Errorf("a re-sync carrying a corrected name staged %d proposals over a "+
			"rejection, want 0 — the memory is keyed on something that moves", got)
	}
}

// A different address against the same lead is a different question.
//
// The other half of the key. A refusal is remembered with no expiry, so a
// target-only key would mean one "no" ends dedupe on that lead permanently: a
// genuinely new collision on a second address would never be raised.
func TestADifferentAddressIsStillProposedAfterARefusal(t *testing.T) {
	e := setupMerge(t)
	incumbent := e.capturedLead(t, "apollo", "a-1", "Dana Dup", "dana@example.test")
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	e.rejectMerge(t, incumbent)

	// The rep adds that second address to the incumbent themselves, and a
	// connector then captures a record carrying it.
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE lead SET email = 'dana.new@example.test' WHERE id = $1`, incumbent); err != nil {
		t.Fatalf("moving the incumbent's address: %v", err)
	}
	e.capturedLead(t, "hubspot", "h-10", "Dana Duplicate", "dana.new@example.test")
	if got := e.pendingMerges(t, incumbent); got != 1 {
		t.Errorf("a collision on an address nobody refused staged %d proposals, want 1 — "+
			"one refusal has ended dedupe on this lead for good", got)
	}
}

// A refusal on one lead says nothing about another.
//
// This is how a too-wide key fails, and it is the failure that hides: dedupe
// quietly stops raising proposals and every other test still passes, because
// each one seeds its own lead.
func TestARejectedMergeOnOneLeadDoesNotSilenceAnother(t *testing.T) {
	e := setupMerge(t)
	declined := e.capturedLead(t, "apollo", "a-1", "Dana Dup", "dana@example.test")
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	e.rejectMerge(t, declined)

	other := e.capturedLead(t, "apollo", "a-2", "Sam Sample", "sam@example.test")
	e.capturedLead(t, "hubspot", "h-11", "Samuel Sample", "sam@example.test")
	if got := e.pendingMerges(t, other); got != 1 {
		t.Errorf("a lead nobody refused has %d merge proposals, want 1 — one "+
			"rejection has silenced a lead it was never about", got)
	}
	if got := e.pendingMerges(t, declined); got != 0 {
		t.Errorf("the refused lead was asked again: %d pending", got)
	}
}

// The same address in different capitalisation is one question.
//
// Providers do not agree on case, and the collision itself is found with
// `email = lower($1)` — so two syncs differing only in capitalisation hit the
// same lead. An identity carrying the raw spelling would read them as two
// questions and forget a refusal the first time a provider changed its mind
// about case.
func TestARejectedMergeStaysRefusedWhenTheAddressCaseChanges(t *testing.T) {
	e := setupMerge(t)
	incumbent := e.capturedLead(t, "apollo", "a-1", "Dana Dup", "dana@example.test")
	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "dana@example.test")
	e.rejectMerge(t, incumbent)

	e.capturedLead(t, "hubspot", "h-9", "Dana Duplicate", "  DANA@Example.TEST ")
	if got := e.pendingMerges(t, incumbent); got != 0 {
		t.Errorf("the same address in another case staged %d proposals over a "+
			"rejection, want 0", got)
	}
}
