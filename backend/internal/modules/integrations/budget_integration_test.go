// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// Reconciliation settles a hold against what the provider actually charged,
// and the billing basis decides what a no-match does to it. Both arms are
// money: releasing a hold the provider kept understates the bill, and holding
// one it refunded silently shrinks the customer's ceiling for the month.

import (
	"context"
	"strconv"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// reserveOneRun queues a run through the real pipeline and returns its id, so
// the reservations under test are the ones production writes.
func reserveOneRun(t *testing.T, e *runsEnv) string {
	t.Helper()
	run, err := e.store.QueueRun(e.ctx, provider.QueueInput{
		PersonID: e.mine.String(), Provider: "surfe", Trigger: provider.TriggerManual,
	})
	if err != nil {
		t.Fatal(err)
	}
	if run.State != provider.RunQueued {
		t.Fatalf("run is %s, want queued", run.State)
	}
	return run.ID
}

func creditsFor(t *testing.T, e *runsEnv, runID, pool string) (reserved int, actual *int) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(),
		`SELECT reserved_credits, actual_credits FROM provider_run_reservation
		  WHERE run_id = $1 AND pool = $2`, runID, pool).Scan(&reserved, &actual); err != nil {
		t.Fatal(err)
	}
	return reserved, actual
}

// charged prints what a reader needs to act on: the number, or the fact that
// the row was never reconciled at all — which is a different failure from
// being charged the wrong amount, and `%v` on the pointer prints an address
// that distinguishes neither.
func charged(actual *int) string {
	if actual == nil {
		return "unreconciled"
	}
	return strconv.Itoa(*actual)
}

// A provider that charges only for a successful result owes nothing on a
// no-match, so the whole hold is released.
func TestReconcileReleasesTheHoldWhenNothingWasBought(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	runID := reserveOneRun(t, e)
	desc := NewOfflineProvider(0, e.store.now).Descriptor()

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.reconcile(e.ctx, tx, desc, runID, nil, false)
	}); err != nil {
		t.Fatal(err)
	}

	_, actual := creditsFor(t, e, runID, "email")
	if actual == nil || *actual != 0 {
		t.Errorf("actual_credits = %s, want 0 — a per-successful-result provider refunds a no-match, and holding those credits would shrink the customer's ceiling for nothing", charged(actual))
	}
}

// On a match, the provider's own number wins where it gave one.
func TestReconcileRecordsWhatTheProviderActuallyCharged(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	runID := reserveOneRun(t, e)
	desc := NewOfflineProvider(0, e.store.now).Descriptor()

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.reconcile(e.ctx, tx, desc, runID,
			map[provider.Pool]int{"email": 1, "mobile": 1}, true)
	}); err != nil {
		t.Fatal(err)
	}

	for _, pool := range []string{"email", "mobile"} {
		_, actual := creditsFor(t, e, runID, pool)
		if actual == nil || *actual != 1 {
			t.Errorf("%s actual_credits = %s, want 1", pool, charged(actual))
		}
	}
}

// A run can match on one category and find nothing on another, and the pool it
// found nothing in must not be charged by a provider that bills per successful
// result.
//
// This is the case a customer meets by BUYING one detail: press "buy mobile" on
// a contact whose employment is already known, the run completes, and the mobile
// pool is silent because the provider had no number. The run is a match, so the
// whole-run release never fires — and the silent pool fell through to its own
// hold. A credit for a number nobody received.
func TestReconcileReleasesAPoolTheProviderNeverAnsweredFor(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	runID := reserveOneRun(t, e)
	desc := NewOfflineProvider(0, e.store.now).Descriptor()
	// Stated, not assumed: under any other basis this case asserts the opposite
	// of what is owed, and it would be asserting it silently.
	if desc.Billing != provider.BillingPerSuccessfulResult {
		t.Fatalf("billing is %s, want per_successful_result", desc.Billing)
	}

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		// A match — the email pool answered and is owed. The mobile pool is
		// absent from the report, which under this basis IS its no-match.
		return e.store.reconcile(e.ctx, tx, desc, runID,
			map[provider.Pool]int{"email": 1}, true)
	}); err != nil {
		t.Fatal(err)
	}

	if _, actual := creditsFor(t, e, runID, "email"); actual == nil || *actual != 1 {
		t.Errorf("email actual_credits = %s, want 1 — the pool that answered is owed", charged(actual))
	}
	_, actual := creditsFor(t, e, runID, "mobile")
	if actual == nil || *actual != 0 {
		t.Errorf("mobile actual_credits = %s, want 0: this provider charges only for a "+
			"match, so a pool it said nothing about owes nothing", charged(actual))
	}
}

// The other arm of the same rule, and the reason it is a rule rather than an
// unconditional release: a vendor that charges per REQUEST has already billed
// for the call whether or not it found anything. Releasing that hold would tell
// the customer they have credits they do not, and the overspend surfaces as a
// refused run later in the month with no explanation on this screen.
//
// The descriptor is built here because no adapter in this tree bills that way
// yet. reconcile takes one as an argument, so this exercises the real function
// against the basis it is asked to honour rather than a stub of it — and the
// day an adapter does declare per_request, this is the case that already held
// the behaviour it needs.
func TestReconcileKeepsAPerRequestHoldTheProviderNeverAnsweredFor(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	runID := reserveOneRun(t, e)
	desc := NewOfflineProvider(0, e.store.now).Descriptor()
	desc.Billing = provider.BillingPerRequest

	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return e.store.reconcile(e.ctx, tx, desc, runID,
			map[provider.Pool]int{"email": 1}, true)
	}); err != nil {
		t.Fatal(err)
	}

	reserved, actual := creditsFor(t, e, runID, "mobile")
	if actual == nil || *actual != reserved {
		t.Errorf("mobile actual_credits = %s, want the reserved %d: this vendor charged for "+
			"the request, so there is no refund to pass on", charged(actual), reserved)
	}
}
