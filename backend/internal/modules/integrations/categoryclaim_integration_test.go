// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// A category is bought at most once at a time per contact and provider. The
// live-run fingerprint index only catches a repeat of the whole set. Two runs
// whose sets overlap without matching are refused by the per-category claim.

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

func (e *runsEnv) queueCategories(t *testing.T, cats ...provider.Category) provider.Run {
	t.Helper()
	run, err := e.store.QueueRun(e.ctx, provider.QueueInput{
		ContactID: e.mine.String(), Provider: e.provider, Trigger: provider.TriggerManual,
		Categories: cats,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}

func (e *runsEnv) heldCategories(t *testing.T, runID string) []string {
	t.Helper()
	var held []string
	if err := e.owner.QueryRow(context.Background(), `
		SELECT coalesce(array_agg(category ORDER BY category), '{}')
		  FROM provider_run_category WHERE run_id = $1`, runID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	return held
}

func (e *runsEnv) reservationsOf(t *testing.T, runID string) int {
	t.Helper()
	var held int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM provider_run_reservation WHERE run_id = $1`, runID).Scan(&held); err != nil {
		t.Fatal(err)
	}
	return held
}

// A button buying {email, mobile} pressed while a lookup for {email} is still
// live would buy the shared email twice. It is refused whole, as a skipped run
// that holds no credit and no claim.
func TestARunSharingALiveCategoryIsSkippedWhole(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	first := e.queueCategories(t, "professional_email")
	if first.State != provider.RunQueued {
		t.Fatalf("the first run is %s, want queued", first.State)
	}

	second := e.queueCategories(t, "mobile", "professional_email")
	if second.State != provider.RunSkipped || second.SkipReason != provider.SkipCategoryInFlight {
		t.Fatalf("the overlapping run is %s/%s, want skipped/category_in_flight — both would buy professional_email",
			second.State, second.SkipReason)
	}
	if second.ID == first.ID {
		t.Fatal("the overlapping run was handed back as the run in flight, so the mobile it asked for is silently dropped")
	}
	if n := e.reservationsOf(t, second.ID); n != 0 {
		t.Errorf("the refused run holds %d reservations, want none: it will never spend", n)
	}
	if held := e.heldCategories(t, second.ID); len(held) != 0 {
		t.Errorf("the refused run still claims %v, which would block the next request for those categories", held)
	}
	if held := e.heldCategories(t, first.ID); strings.Join(held, ",") != "professional_email" {
		t.Errorf("the live run claims %v, want [professional_email]", held)
	}
	if e.enqueued != 1 {
		t.Errorf("%d submit jobs enqueued, want 1: the refused run must never reach the provider", e.enqueued)
	}
	var audited int
	if err := e.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM audit_log
		 WHERE entity_type = 'provider_run' AND entity_id = $1 AND action = 'create'`,
		second.ID).Scan(&audited); err != nil {
		t.Fatal(err)
	}
	if audited != 1 {
		t.Errorf("the refused run has %d audit rows, want 1: a skip is a decision somebody's request caused", audited)
	}
}

// A run admitted before claim rows existed holds none, so the unique index
// cannot see it. Its requested categories still refuse an overlapping request.
func TestARunQueuedBeforeClaimsExistedStillRefusesAnOverlap(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	// Written by hand because it models a row no current writer produces: a
	// live run from before the claim table, which never got claim rows.
	var legacy string
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO provider_run
		  (subject_kind, contact_id, provider, trigger, state, input_fingerprint,
		   external_correlation_id, connection_version, connection_epoch,
		   configuration_snapshot, requested_categories)
		VALUES ('contact', $1, $2, 'manual', 'in_progress', 'fp-legacy',
		        gen_random_uuid(), 1, 1, '{}'::jsonb, ARRAY['professional_email'])
		RETURNING id::text`, e.mine, e.provider).Scan(&legacy); err != nil {
		t.Fatal(err)
	}

	run := e.queueCategories(t, "professional_email", "mobile")
	if run.State != provider.RunSkipped || run.SkipReason != provider.SkipCategoryInFlight {
		t.Errorf("the request overlapping an unclaimed live run is %s/%s, want skipped/category_in_flight",
			run.State, run.SkipReason)
	}
}

// Two requests racing on one contact: whichever commits first is admitted and
// the other is refused, never both.
func TestConcurrentOverlappingRunsAdmitOnlyOne(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	sets := [][]provider.Category{
		{"professional_email"},
		{"professional_email", "mobile"},
	}
	runs := make([]provider.Run, len(sets))
	errs := make([]error, len(sets))
	var wg sync.WaitGroup
	for i, cats := range sets {
		wg.Go(func() {
			runs[i], errs[i] = e.store.QueueRun(e.ctx, provider.QueueInput{
				ContactID: e.mine.String(), Provider: e.provider, Trigger: provider.TriggerManual,
				Categories: cats,
			})
		})
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	queued, refused := 0, 0
	for _, run := range runs {
		switch {
		case run.State == provider.RunQueued:
			queued++
		case run.State == provider.RunSkipped && run.SkipReason == provider.SkipCategoryInFlight:
			refused++
		}
	}
	if queued != 1 || refused != 1 {
		t.Errorf("%d queued and %d refused, want 1 and 1: two live runs both buying professional_email is the duplicate charge", queued, refused)
	}
}

// The guard is per category, not per contact: two purchases that share
// nothing are both admitted.
func TestRunsWithDisjointCategoriesAreBothAdmitted(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	if _, err := e.owner.Exec(context.Background(), `
		UPDATE provider_connection
		   SET categories = ARRAY['professional_email','mobile','linkedin_profile']
		 WHERE provider = $1`, e.provider); err != nil {
		t.Fatal(err)
	}
	// The connection is one row per provider for the whole database, so the
	// wider selection is put back for the tests that follow.
	t.Cleanup(func() {
		if _, err := e.owner.Exec(context.Background(), `
			UPDATE provider_connection SET categories = ARRAY['professional_email','mobile']
			 WHERE provider = $1`, e.provider); err != nil {
			t.Errorf("restoring the connection's categories: %v", err)
		}
	})
	email := e.queueCategories(t, "professional_email")
	profile := e.queueCategories(t, "linkedin_profile")
	if email.State != provider.RunQueued || profile.State != provider.RunQueued {
		t.Errorf("runs are %s/%s and %s/%s, want both queued — they share no category",
			email.State, email.SkipReason, profile.State, profile.SkipReason)
	}
}

// An identical repeat is still answered with the run in flight, as before:
// the fingerprint index catches it ahead of the category claim.
func TestAnIdenticalRepeatStillReturnsTheRunInFlight(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	first := e.queueCategories(t, "professional_email", "mobile")
	repeat := e.queueCategories(t, "mobile", "professional_email")
	if repeat.ID != first.ID || repeat.State != provider.RunQueued {
		t.Errorf("the repeat is run %s in %s, want the first run %s still queued", repeat.ID, repeat.State, first.ID)
	}
	if e.enqueued != 1 {
		t.Errorf("%d submit jobs enqueued, want 1", e.enqueued)
	}
}

// Claims exist only while their run is live. A run finished through the real
// submit and poll writers frees its categories. Asking again is then a new
// purchase, not a refusal.
func TestAFinishedRunReleasesItsCategories(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	sealCredential(t, e)
	first := e.queueCategories(t, "professional_email")
	if err := e.store.ExecuteSubmit(e.ctx, first.ID); err != nil {
		t.Fatal(err)
	}
	if held := e.heldCategories(t, first.ID); len(held) != 1 {
		t.Fatalf("the in-progress run claims %v, want [professional_email]: it is still live", held)
	}
	// No claim writer is bound, so the hand-off after completion fails; the
	// completion itself is what this test needs.
	if err := e.store.RunDueSweep(e.ctx); err == nil || !strings.Contains(err.Error(), "no claim writer is bound") {
		t.Fatalf("the sweep did not reach the hand-off: %v", err)
	}
	if state, _, _ := runRow(t, e, first.ID); state != string(provider.RunCompleted) {
		t.Fatalf("the run is %s, want completed", state)
	}
	if held := e.heldCategories(t, first.ID); len(held) != 0 {
		t.Errorf("the completed run still claims %v, so no one could buy that category again", held)
	}

	again := e.queueCategories(t, "professional_email", "mobile")
	if again.State != provider.RunQueued {
		t.Errorf("asking again after completion is %s/%s, want queued", again.State, again.SkipReason)
	}
}
