// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"fmt"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ambiguousLeadWithTwoContacts builds a lead one contact holds the email of
// and another is linked to. A live email sits on one contact only, so a lead
// matches two contacts through different lanes.
func ambiguousLeadWithTwoContacts(t *testing.T, e *integration.Env, tag string) (ids.LeadID, ids.ContactID, ids.ContactID) {
	t.Helper()
	email := tag + "@pairs.example.test"
	lead := repairLead(t, e, tag+" Lead", &email)
	linked := repairContact(t, e, tag+" Linked")
	holder := repairContact(t, e, tag+" Holder")
	e.WsExec(t, `INSERT INTO contact_email (contact_id, email, email_type, is_primary, position, source, captured_by)
		VALUES ($1, $2, 'work', true, 0, 'manual', 'human:test')`, holder, email)
	e.WsExec(t, `UPDATE lead SET from_contact_id = $2 WHERE id = $1`, lead, linked)
	return lead, linked, holder
}

// runLeadPairsBatch is one run of the cleanup from the start of the lead list.
func runLeadPairsBatch(ctx context.Context, t *testing.T, e *integration.Env) datatask.BatchOut {
	t.Helper()
	step := leadContactPairsDataTask(e.Pool).Steps[0]
	var out datatask.BatchOut
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		out, err = step.Batch(ctx, tx, datatask.BatchIn{Limit: 25, RunID: ids.NewV7()})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func leadPairsEstimate(ctx context.Context, t *testing.T, e *integration.Env) int64 {
	t.Helper()
	var count int64
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		var err error
		count, _, err = leadContactPairsDataTask(e.Pool).Steps[0].Estimate(ctx, tx, 100)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return count
}

func contactPairID(t *testing.T, e *integration.Env, a, b ids.ContactID) ids.UUID {
	t.Helper()
	id, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM dedupe_candidate WHERE entity_type = 'contact'
		AND $1 IN (left_contact_id, right_contact_id) AND $2 IN (left_contact_id, right_contact_id)`, a, b))
	if err != nil {
		t.Fatalf("reading the filed contact pair: %v", err)
	}
	return id
}

func TestAnAmbiguousLeadFilesItsContactsForReviewAndMergesOnceTheyAreOne(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead, first, second := ambiguousLeadWithTwoContacts(t, e, "merge")

	out := runLeadPairsBatch(ctx, t, e)
	if out.Changed != 0 {
		t.Errorf("the ambiguous lead changed %d rows, want none", out.Changed)
	}
	pair := contactPairID(t, e, first, second)
	if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE id = $1 AND disposition = 'open'
		AND source = 'lead_contact_cleanup'`, pair); got != 1 {
		t.Fatalf("the pair is not open on the review queue")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL`, lead); got != 1 {
		t.Error("the cleanup changed the lead before the contacts were merged")
	}

	runLeadPairsBatch(ctx, t, e)
	if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'contact'`); got != 1 {
		t.Errorf("a second run filed %d pairs, want the one it already holds", got)
	}

	keep := first.UUID
	if _, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), pair, "merge", &keep); err != nil {
		t.Fatal(err)
	}
	out = runLeadPairsBatch(ctx, t, e)
	if out.Changed != 1 {
		t.Errorf("the run after the merge changed %d leads, want 1", out.Changed)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND status = 'promoted'
		AND archived_at IS NOT NULL AND promoted_contact_id = $2`, lead, first); got != 1 {
		t.Error("the lead was not merged into the surviving contact")
	}
}

func TestADismissedPairLeavesTheLeadAloneAndOutOfTheCleanup(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead, first, second := ambiguousLeadWithTwoContacts(t, e, "dismiss")

	runLeadPairsBatch(ctx, t, e)
	if got := leadPairsEstimate(ctx, t, e); got != 1 {
		t.Fatalf("an open pair leaves %d leads in the cleanup, want 1", got)
	}
	if _, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), contactPairID(t, e, first, second), "not_a_duplicate", nil); err != nil {
		t.Fatal(err)
	}

	out := runLeadPairsBatch(ctx, t, e)
	if out.Scanned != 0 || out.Changed != 0 {
		t.Errorf("the run after the dismissal scanned %d and changed %d, want neither", out.Scanned, out.Changed)
	}
	if got := leadPairsEstimate(ctx, t, e); got != 0 {
		t.Errorf("a dismissed pair leaves %d leads in the cleanup, want none", got)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM lead WHERE id = $1 AND archived_at IS NULL AND status <> 'promoted'`, lead); got != 1 {
		t.Error("the lead stopped being a lead")
	}
}

func TestALeadMatchingThreeContactsStaysUntilEveryPairIsDismissed(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead, first, second := ambiguousLeadWithTwoContacts(t, e, "three")
	third := repairContact(t, e, "three Third")
	e.WsExec(t, `UPDATE contact SET converted_from_lead_id = $2 WHERE id = $1`, third, lead)

	runLeadPairsBatch(ctx, t, e)
	if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'contact'`); got != 3 {
		t.Fatalf("three contacts filed %d pairs, want 3", got)
	}
	for _, pair := range [][2]ids.ContactID{{first, second}, {first, third}} {
		if _, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), contactPairID(t, e, pair[0], pair[1]), "not_a_duplicate", nil); err != nil {
			t.Fatal(err)
		}
	}
	if got := leadPairsEstimate(ctx, t, e); got != 1 {
		t.Errorf("two of three pairs dismissed leaves %d leads in the cleanup, want 1", got)
	}
	if _, err := e.Contacts.DisposeDedupeCandidate(e.Admin(), contactPairID(t, e, second, third), "not_a_duplicate", nil); err != nil {
		t.Fatal(err)
	}
	if got := leadPairsEstimate(ctx, t, e); got != 0 {
		t.Errorf("every pair dismissed leaves %d leads in the cleanup, want none", got)
	}
}

func TestALeadMatchingMoreContactsThanTheCapFilesNothingAndStaysInScope(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	lead := repairLead(t, e, "Crowded Lead", nil)
	e.WsExec(t, `UPDATE lead SET linkedin_url = 'https://www.linkedin.com/in/crowded' WHERE id = $1`, lead)
	for i := range 11 {
		contact := repairContact(t, e, fmt.Sprintf("Crowded %02d", i))
		e.WsExec(t, `INSERT INTO contact_social (contact_id, platform, handle)
			VALUES ($1, 'linkedin', 'https://www.linkedin.com/in/crowded')`, contact)
	}

	out := runLeadPairsBatch(ctx, t, e)
	if out.Changed != 0 {
		t.Errorf("the crowded lead changed %d rows, want none", out.Changed)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM dedupe_candidate WHERE entity_type = 'contact'`); got != 0 {
		t.Errorf("the lead over the cap filed %d pairs, want none", got)
	}
	if got := leadPairsEstimate(ctx, t, e); got != 1 {
		t.Errorf("the lead over the cap leaves %d leads in the cleanup, want 1", got)
	}
}
