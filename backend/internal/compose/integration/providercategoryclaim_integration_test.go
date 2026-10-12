// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The per-category admission claims across a merge and an erasure, the two
// paths that move or detach a contact's runs. Both rewrite provider_run's
// contact_id under a run that may still be live. The claims follow that key,
// so each path must leave them consistent rather than fail.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedCategoryClaims writes a live run's admission claims by hand, for the
// reason seedRun gives. These tests are about what happens to a live run once
// it exists, not about how QueueRun admits it.
func seedCategoryClaims(t *testing.T, e *Env, runID string, contactID ids.UUID, categories ...string) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := tx.Exec(context.Background(), `
			INSERT INTO provider_run_category (run_id, contact_id, provider, category)
			SELECT $1, $2, 'surfe', unnest($3::text[])`, runID, contactID, categories)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

// claimedBy maps each category claimed on a contact to the run holding it.
func claimedBy(t *testing.T, e *Env, contactID ids.UUID) map[string]string {
	t.Helper()
	held := map[string]string{}
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(),
			`SELECT category, run_id::text FROM provider_run_category WHERE contact_id = $1`, contactID)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var category, runID string
			if err := rows.Scan(&category, &runID); err != nil {
				return err
			}
			held[category] = runID
		}
		return rows.Err()
	}); err != nil {
		t.Fatal(err)
	}
	return held
}

// Both sides of a merge hold a live run claiming professional_email. Both
// runs were admitted and either may have been charged, so both are kept. The
// survivor keeps its claim on the shared category. The merged-away run keeps
// its claim on what only it was buying.
func TestMergeKeepsBothLiveRunsAndDropsOnlyTheCollidingClaim(t *testing.T) {
	e := Setup(t)
	survivor := seedMergeSubject(t, e, "Dora Survivor")
	source := seedMergeSubject(t, e, "Dora Source")
	survivorRun := seedRun(t, e, survivor, "submitting", "fp-survivor-email", false)
	sourceRun := seedRun(t, e, source, "in_progress", "fp-source-email-mobile", false)
	seedCategoryClaims(t, e, survivorRun, survivor, "professional_email")
	seedCategoryClaims(t, e, sourceRun, source, "professional_email", "mobile")

	store := contacts.NewStore(e.DB())
	if _, err := store.MergeContact(e.Admin(), ids.From[ids.ContactKind](source),
		ids.From[ids.ContactKind](survivor), nil); err != nil {
		t.Fatalf("the merge failed with both sides holding a live claim on one category: %v", err)
	}

	if state, _ := runState(t, e, survivorRun); state != "submitting" {
		t.Errorf("the survivor's run is %s, want submitting: the merge must not touch it", state)
	}
	if state, _ := runState(t, e, sourceRun); state != "in_progress" {
		t.Errorf("the merged-away record's run is %s, want in_progress: it may already have been charged", state)
	}
	held := claimedBy(t, e, survivor)
	if held["professional_email"] != survivorRun {
		t.Errorf("professional_email is claimed by %q, want the survivor's run %s", held["professional_email"], survivorRun)
	}
	if held["mobile"] != sourceRun {
		t.Errorf("mobile is claimed by %q, want the merged-away run %s, which is still buying it", held["mobile"], sourceRun)
	}
	if left := claimedBy(t, e, source); len(left) != 0 {
		t.Errorf("claims %v still name the merged-away record", left)
	}
}

// A survivor run still queued would buy again what a merged-away run past
// queued is already buying. It never reached the provider, so the merge skips
// it and the run further along keeps the claim.
func TestMergeSkipsAQueuedRunBuyingWhatTheOtherSideIsBuying(t *testing.T) {
	e := Setup(t)
	survivor := seedMergeSubject(t, e, "Erik Survivor")
	source := seedMergeSubject(t, e, "Erik Source")
	queuedRun := seedRun(t, e, survivor, "queued", "fp-survivor-email-mobile", false)
	sourceRun := seedRun(t, e, source, "in_progress", "fp-source-email", false)
	seedCategoryClaims(t, e, queuedRun, survivor, "mobile", "professional_email")
	seedCategoryClaims(t, e, sourceRun, source, "professional_email")

	store := contacts.NewStore(e.DB())
	if _, err := store.MergeContact(e.Admin(), ids.From[ids.ContactKind](source),
		ids.From[ids.ContactKind](survivor), nil); err != nil {
		t.Fatal(err)
	}

	var state, reason string
	var audited int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		if err := tx.QueryRow(context.Background(),
			`SELECT state, coalesce(skip_reason, '') FROM provider_run WHERE id = $1`,
			queuedRun).Scan(&state, &reason); err != nil {
			return err
		}
		return tx.QueryRow(context.Background(), `
			SELECT count(*) FROM audit_log
			 WHERE entity_type = 'provider_run' AND entity_id = $1 AND action = 'update'`,
			queuedRun).Scan(&audited)
	}); err != nil {
		t.Fatal(err)
	}
	if state != "skipped" || reason != "category_in_flight" {
		t.Errorf("the survivor's queued run is %s/%q, want skipped/category_in_flight: it would buy professional_email a second time", state, reason)
	}
	if audited != 1 {
		t.Errorf("the skipped run has %d update audit rows, want 1", audited)
	}
	if st, _ := runState(t, e, sourceRun); st != "in_progress" {
		t.Errorf("the merged-away record's run is %s, want in_progress", st)
	}
	held := claimedBy(t, e, survivor)
	if len(held) != 1 || held["professional_email"] != sourceRun {
		t.Errorf("claims are %v, want only professional_email held by the run past queued %s", held, sourceRun)
	}
}

// Erasure detaches a run that may still be live. Its claims go with the
// subject, instead of the scrub failing on a claim that would name nobody.
func TestErasureReleasesALiveRunsCategoryClaims(t *testing.T) {
	e := Setup(t)
	contactID := seedSubject(t, e)
	runID := seedRun(t, e, contactID, "in_progress", "fp-live-erasure", false)
	seedCategoryClaims(t, e, runID, contactID, "professional_email")

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), contactID, "test"); err != nil {
		t.Fatalf("the erasure failed on a live run holding a category claim: %v", err)
	}

	var left int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM provider_run_category WHERE run_id = $1`, runID).Scan(&left)
	}); err != nil {
		t.Fatal(err)
	}
	if left != 0 {
		t.Errorf("%d category claims survive the erasure of their subject", left)
	}
}
