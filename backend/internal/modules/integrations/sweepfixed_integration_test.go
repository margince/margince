// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integrations

// What the sweep reserves for contacts a user just fixed, proven through the
// real selector against a database holding a long backlog.

import (
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/provider"
)

// plantOlderBacklog adds contacts nobody asked about, all created before any
// other contact in the test. It archives them when the test ends, so the next
// test's selection is not crowded by them.
func (e *runsEnv) plantOlderBacklog(t *testing.T, n int) {
	t.Helper()
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO contact (full_name, first_name, source, captured_by, created_at)
		SELECT 'Backlog Subject', 'Backlog', 'test', 'test:sweepfixed',
		       now() - interval '30 days' - make_interval(secs => g)
		  FROM generate_series(1, $1) g`, n); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := e.owner.Exec(e.ctx,
			`UPDATE contact SET archived_at = now() WHERE captured_by = 'test:sweepfixed'`); err != nil {
			t.Error(err)
		}
	})
}

// plantFixed adds a contact skipped for no identifiers whose own record then
// changed, the way adding a profile link changes it.
func (e *runsEnv) plantFixed(t *testing.T) ids.ContactID {
	t.Helper()
	id := e.plantSubjectSkippedFor(t, provider.SkipNoIdentifiers)
	if _, err := e.owner.Exec(e.ctx,
		`UPDATE contact SET first_name = 'Fixed' WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { e.archive(t, id) })
	return id
}

func (e *runsEnv) archive(t *testing.T, id ids.ContactID) {
	t.Helper()
	if _, err := e.owner.Exec(e.ctx, `UPDATE contact SET archived_at = now() WHERE id = $1`, id); err != nil {
		t.Error(err)
	}
}

// tickPicks is what one tick with the given budget queues, in order.
func (e *runsEnv) tickPicks(t *testing.T, budget int) []ids.ContactID {
	t.Helper()
	var chosen []string
	if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		var err error
		chosen, err = e.store.uncoveredSubjects(e.ctx, tx, e.provider, budget)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	out := make([]ids.ContactID, 0, len(chosen))
	for _, id := range chosen {
		parsed, err := ids.Parse(id)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, ids.ContactID{UUID: parsed})
	}
	return out
}

func TestAFixedContactBehindAThousandOlderOnesIsPickedInTheNextTick(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 1000)

	// Created first, so by age it sits behind the whole backlog.
	byProfile := e.plantFixed(t)
	byEmployer := e.plantSubjectSkippedFor(t, provider.SkipNoIdentifiers)
	e.linkEmployer(t, byEmployer, false)
	t.Cleanup(func() { e.archive(t, byEmployer) })

	picks := e.tickPicks(t, sweepTickBudget)

	if len(picks) != sweepTickBudget {
		t.Fatalf("a tick picked %d contacts, want its whole budget of %d", len(picks), sweepTickBudget)
	}
	if !slices.Contains(picks, byProfile) {
		t.Error("a contact whose profile link was just added was not picked while 1000 older ones waited")
	}
	if !slices.Contains(picks, byEmployer) {
		t.Error("a contact whose employer was just linked was not picked while 1000 older ones waited")
	}
	if picks[0] != byEmployer {
		t.Error("the contact changed last was not first: the newest change must lead")
	}
}

func TestAtMostFivePlacesGoToFixedContactsNewestChangeFirst(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 100)

	var fixed []ids.ContactID
	for range 8 {
		fixed = append(fixed, e.plantFixed(t))
	}

	picks := e.tickPicks(t, sweepTickBudget)

	if len(picks) != sweepTickBudget {
		t.Fatalf("a tick picked %d contacts, want %d: the reservation divides the budget", len(picks), sweepTickBudget)
	}
	want := []ids.ContactID{fixed[7], fixed[6], fixed[5], fixed[4], fixed[3]}
	if !slices.Equal(picks[:sweepFixedShare], want) {
		t.Errorf("the first %d places are not the five newest fixes in newest-first order", sweepFixedShare)
	}
	for _, id := range fixed[:3] {
		if slices.Contains(picks, id) {
			t.Error("a sixth fixed contact took a backlog place ahead of older contacts")
		}
	}
	seen := map[ids.ContactID]bool{}
	for _, id := range picks {
		if seen[id] {
			t.Fatal("a contact was picked twice in one tick")
		}
		seen[id] = true
	}
}

func TestUnusedReservedPlacesGoToTheBacklogInOldestFirstOrder(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 100)
	fixed := e.plantFixed(t)

	picks := e.tickPicks(t, sweepTickBudget)

	if len(picks) != sweepTickBudget || picks[0] != fixed {
		t.Fatalf("got %d picks with the fixed contact first = %v, want %d with the fixed contact first",
			len(picks), picks[0] == fixed, sweepTickBudget)
	}
	var created []string
	if err := e.owner.QueryRow(e.ctx, `
		SELECT array_agg(id::text ORDER BY created_at)
		  FROM (SELECT id, created_at FROM contact
		         WHERE archived_at IS NULL AND merged_into_id IS NULL AND id <> $1
		           AND NOT EXISTS (SELECT 1 FROM provider_run r WHERE r.contact_id = contact.id)
		         ORDER BY created_at LIMIT $2) oldest`,
		fixed, sweepTickBudget-1).Scan(&created); err != nil {
		t.Fatal(err)
	}
	for i, id := range picks[1:] {
		if id.String() != created[i] {
			t.Fatalf("backlog place %d is not the %dth oldest contact: the backlog order changed", i, i+1)
		}
	}
}

func TestAContactThatIsNotFixedKeepsItsCooldown(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 100)
	unfixed := e.plantSubjectSkippedFor(t, provider.SkipNoIdentifiers)
	t.Cleanup(func() { e.archive(t, unfixed) })

	if slices.Contains(e.tickPicks(t, sweepTickBudget), unfixed) {
		t.Error("a no-identifiers contact nobody changed was picked inside its cooldown")
	}
}

func TestTheFixedShareScalesWithTheBudgetAndTheBacklogKeepsTheRest(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 30)
	fixed := map[ids.ContactID]bool{}
	for range 8 {
		fixed[e.plantFixed(t)] = true
	}

	for budget, wantFixed := range map[int]int{1: 0, 2: 0, 4: 0, 5: 1, 6: 1, 10: 2, 25: 5} {
		picks := e.tickPicks(t, budget)
		if len(picks) != budget {
			t.Errorf("a budget of %d picked %d contacts: a daily limit must still cap the total", budget, len(picks))
		}
		var got int
		for _, id := range picks {
			if fixed[id] {
				got++
			}
		}
		if got != wantFixed {
			t.Errorf("a budget of %d gave %d places to fixed contacts, want %d", budget, got, wantFixed)
		}
	}
}

// A daily limit that leaves five runs gives a tick five places. Six fresh
// fixes arrive every tick, and the backlog must still drain. A fixed contact
// the run skipped again for lack of identifiers must not come back.
func TestTheBacklogDrainsUnderADailyLimitWhileFixesKeepArriving(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	const backlogSize = 30
	e.plantOlderBacklog(t, backlogSize)
	picked := map[ids.ContactID]int{}
	backlogLeft := backlogSize
	// Five places give the backlog four a tick, so thirty need eight ticks.
	for tick := 1; tick <= 8 && backlogLeft > 0; tick++ {
		for range 6 {
			e.plantFixed(t)
		}
		e.allowFiveMoreRuns(t)
		budget, err := e.sweepBudgetNow(t)
		if err != nil {
			t.Fatal(err)
		}
		for _, id := range e.tickPicks(t, budget) {
			picked[id]++
			if picked[id] > 1 {
				t.Fatalf("a contact was picked again in tick %d after its run was recorded", tick)
			}
			if e.isBacklog(t, id) {
				backlogLeft--
				e.recordRun(t, id, "no_match")
				continue
			}
			e.recordRun(t, id, "skipped")
		}
	}
	if backlogLeft != 0 {
		t.Errorf("%d backlog contacts were never reached: fixes starved the backlog", backlogLeft)
	}
}

// allowFiveMoreRuns sets the daily limit to what today has spent plus five.
// Each tick then sees a budget of five, as a limit leaving five runs would.
func (e *runsEnv) allowFiveMoreRuns(t *testing.T) {
	t.Helper()
	if _, err := e.owner.Exec(e.ctx, `
		UPDATE provider_connection SET daily_run_limit = 5 + (
		       SELECT count(*) FROM provider_run
		        WHERE provider = $1 AND state <> 'skipped' AND state <> 'cancelled'
		          AND created_at >= date_trunc('day', now() AT TIME ZONE 'UTC'))
		 WHERE provider = $1`, e.provider); err != nil {
		t.Fatal(err)
	}
}

func (e *runsEnv) isBacklog(t *testing.T, id ids.ContactID) bool {
	t.Helper()
	var backlog bool
	if err := e.owner.QueryRow(e.ctx,
		`SELECT captured_by = 'test:sweepfixed' FROM contact WHERE id = $1`, id).Scan(&backlog); err != nil {
		t.Fatal(err)
	}
	return backlog
}

// recordRun writes what the sweep leaves behind for a picked contact: an
// answer, or a second no_identifiers skip.
func (e *runsEnv) recordRun(t *testing.T, id ids.ContactID, state string) {
	t.Helper()
	var reason *string
	if state == "skipped" {
		r := string(provider.SkipNoIdentifiers)
		reason = &r
	}
	if _, err := e.owner.Exec(e.ctx, `
		INSERT INTO provider_run
		       (contact_id, subject_kind, provider, trigger, state, skip_reason,
		        connection_version, connection_epoch, configuration_snapshot,
		        requested_categories, input_fingerprint, external_correlation_id)
		VALUES ($1, 'contact', $2, 'automatic_backfill', $3, $4, 1, 1, '{}'::jsonb,
		        ARRAY['linkedin_profile'], $5, gen_random_uuid())`,
		id, e.provider, state, reason, "fp-tick-"+id.String()+"-"+state); err != nil {
		t.Fatal(err)
	}
}

func TestTheSettingsCountLeavesOutARestrictedContact(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 3)
	count := func() int {
		var out BacklogCount
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			var err error
			out, err = e.store.backlogInTx(e.ctx, tx, e.provider)
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return out.Remaining
	}
	before := count()
	var victim ids.ContactID
	if err := e.owner.QueryRow(e.ctx,
		`SELECT id FROM contact WHERE captured_by = 'test:sweepfixed' AND archived_at IS NULL LIMIT 1`).Scan(&victim); err != nil {
		t.Fatal(err)
	}
	e.restrict(t, victim)
	if after := count(); after != before-1 {
		t.Errorf("the count went from %d to %d after a restriction, want %d: the sweep can never pick a restricted contact",
			before, after, before-1)
	}
}
