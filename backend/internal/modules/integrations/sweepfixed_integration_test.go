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

func TestTheReservationNeverExceedsTheBudget(t *testing.T) {
	e := setupRuns(t, runsConfig{})
	e.plantOlderBacklog(t, 20)
	for range 8 {
		e.plantFixed(t)
	}

	for _, budget := range []int{1, 2, 5, 6} {
		if got := len(e.tickPicks(t, budget)); got != budget {
			t.Errorf("a budget of %d picked %d contacts: a daily limit must still cap the total", budget, got)
		}
	}
}
