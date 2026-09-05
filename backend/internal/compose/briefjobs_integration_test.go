// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The overnight assembly, driven through the real worker.
//
// What the store's own tests cannot reach is WHO the pass decides to assemble
// for, and that decision is the whole security and product surface of the job:
// it binds each rep's own authority rather than a system principal, it waits
// for the local morning rather than a UTC hour, and it refuses a workspace
// whose deals are not in these tables at all.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// briefJobEnv is one workspace with the overnight worker wired exactly as
// compose wires it, plus a clock the test moves.
type briefJobEnv struct {
	*integration.Env
	worker *briefGenerateWorker
	now    time.Time
}

func setupBriefJob(t *testing.T) *briefJobEnv {
	t.Helper()
	e := integration.Setup(t)
	b := &briefJobEnv{Env: e}
	// Real role rows, because the job resolves each rep's authority from the
	// database rather than from a principal a caller handed it — that IS the
	// job's security argument, and a fixture supplying permissions in memory
	// would prove nothing about it. Deal read is what the ranking needs;
	// installation-settings read is what the base-currency and timezone lookups need.
	b.grantEveryRepTheBriefsReads(t)
	b.worker = &briefGenerateWorker{
		engine: briefs.NewBriefEngine(e.Pool, people.NewStore(InstallationDB(e.Pool))),
		pool:   e.Pool,
		users:  identity.NewService(e.Pool),
		log:    slog.Default(),
		now:    func() time.Time { return b.now },
	}
	return b
}

// grantEveryRepTheBriefsReads gives the fixture's three humans the role a real
// rep holds for this pass: reading deals, and reading the installation settings
// the ranking normalizes money and days against.
func (b *briefJobEnv) grantEveryRepTheBriefsReads(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	owner := integration.OwnerConn(t)
	var roleID ids.UUID
	if err := owner.QueryRow(ctx,
		`INSERT INTO role (key, name, permissions)
		 VALUES ('brief_reader', 'Brief Reader',
		         '{"objects":{"deal":{"read":true},"installation_settings":{"read":true}},"row_scope":"all"}'::jsonb)
		 RETURNING id`).Scan(&roleID); err != nil {
		t.Fatalf("seeding the brief reader role: %v", err)
	}
	// The three reps, deliberately not the fixture's admin seat: that seat holds
	// no role at all, which is what makes it the standing case for a live full
	// seat the pass finds and cannot assemble for.
	for _, rep := range []ids.UUID{b.Rep1, b.Rep2, b.Rep3} {
		if _, err := owner.Exec(ctx,
			`INSERT INTO role_assignment (role_id, user_id) VALUES ($1, $2)`, roleID, rep); err != nil {
			t.Fatalf("assigning the brief reader role to %s: %v", rep, err)
		}
	}
}

// run drives the worker's per-workspace turn, which is what River's row now
// walks rather than what it carries: the pass takes no workspace in its args
// (ADR-0103), so Work would enumerate the fleet and lose the one this suite is
// about. The workspace binding and the clock read are the production ones
// either way — assembleOneWorkspace is what Work calls per tenant.
func (b *briefJobEnv) run(t *testing.T) error {
	t.Helper()
	return b.worker.assembleOneWorkspace(context.Background(), b.WS)
}

// runsFor counts the brief runs stored for one rep on one local day.
func (b *briefJobEnv) runsFor(t *testing.T, user ids.UUID, day time.Time) int {
	t.Helper()
	var n int
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT count(*) FROM brief_run WHERE user_id = $1 AND local_day = $2`,
		user, day.Format(time.DateOnly)).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestOvernightPassAssemblesEachRepsMorningExactlyOnce(t *testing.T) {
	b := setupBriefJob(t)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning

	if err := b.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	for _, rep := range []ids.UUID{b.Rep1, b.Rep2, b.Rep3} {
		if n := b.runsFor(t, rep, morning); n != 1 {
			t.Fatalf("rep %s has %d runs for the morning, want 1", rep, n)
		}
	}

	// The hourly tick comes round again inside the same local day. It must
	// find every rep already served and write nothing — twenty-four ticks a day
	// is the design, and one run per rep per day is what makes that safe.
	b.now = morning.Add(3 * time.Hour)
	if err := b.run(t); err != nil {
		t.Fatalf("the second tick of the same morning failed: %v", err)
	}
	if n := b.runsFor(t, b.Rep1, morning); n != 1 {
		t.Fatalf("a later tick on the same day left %d runs, want the one already there", n)
	}

	// The next local day is a new morning, and gets its own run.
	tomorrow := morning.AddDate(0, 0, 1)
	b.now = tomorrow
	if err := b.run(t); err != nil {
		t.Fatalf("the next morning's pass failed: %v", err)
	}
	if n := b.runsFor(t, b.Rep1, tomorrow); n != 1 {
		t.Fatalf("the next morning has %d runs, want 1", n)
	}
}

func TestOvernightPassWaitsForTheLocalMorning(t *testing.T) {
	b := setupBriefJob(t)
	// Before the briefing hour in the installation's own zone. Assembling here
	// would date the run to a morning that has not started, and the rep would
	// open Home to a brief assembled from a day she has not lived yet.
	night := time.Date(2026, 6, 4, 1, 0, 0, 0, time.UTC)
	b.now = night

	if err := b.run(t); err != nil {
		t.Fatalf("the pre-dawn tick failed: %v", err)
	}
	if n := b.runsFor(t, b.Rep1, night); n != 0 {
		t.Fatalf("a tick before the briefing hour wrote %d runs, want none", n)
	}

	// Once the morning arrives, the same rep is assembled.
	b.now = night.Add(8 * time.Hour)
	if err := b.run(t); err != nil {
		t.Fatalf("the morning tick failed: %v", err)
	}
	if n := b.runsFor(t, b.Rep1, night); n != 1 {
		t.Fatalf("the morning tick wrote %d runs, want 1", n)
	}
}

func TestOvernightPassSkipsAWorkspaceWhoseDealsLiveInTheIncumbent(t *testing.T) {
	b := setupBriefJob(t)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning

	// An overlay installation keeps its deals in the incumbent, so a run assembled
	// from these tables would be an empty queue — which reads on the screen
	// exactly like a quiet morning while being a different fact entirely.
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE overlay_mode SET sor_mode = 'overlay', incumbent = 'hubspot'`); err != nil {
		t.Fatal(err)
	}

	if err := b.run(t); err != nil {
		t.Fatalf("the pass over an overlay workspace failed: %v — it must decline, not error", err)
	}
	if n := b.runsFor(t, b.Rep1, morning); n != 0 {
		t.Fatalf("the overlay workspace got %d runs, want none — an empty queue there is a lie, not a quiet morning", n)
	}
}

func TestOvernightPassLeavesReadSeatsAndAgentsWithoutAMorning(t *testing.T) {
	b := setupBriefJob(t)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning
	owner := integration.OwnerConn(t)

	// A read seat cannot act on a deal, and an agent has no morning to prepare.
	// Both would otherwise be assembled a brief nobody reads.
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET seat_type = 'read' WHERE id = $1`, b.Rep2); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(context.Background(),
		`UPDATE app_user SET is_agent = true WHERE id = $1`, b.Rep3); err != nil {
		t.Fatal(err)
	}

	if err := b.run(t); err != nil {
		t.Fatalf("the overnight pass failed: %v", err)
	}
	if n := b.runsFor(t, b.Rep1, morning); n != 1 {
		t.Fatalf("the full seat has %d runs, want 1 — without it this test proves nothing about the two below", n)
	}
	if n := b.runsFor(t, b.Rep2, morning); n != 0 {
		t.Fatalf("the read seat got %d runs, want none", n)
	}
	if n := b.runsFor(t, b.Rep3, morning); n != 0 {
		t.Fatalf("the agent seat got %d runs, want none", n)
	}
}

func TestASeatWithoutADealGrantCostsOnlyItsOwnBrief(t *testing.T) {
	b := setupBriefJob(t)
	morning := time.Date(2026, 6, 4, 7, 0, 0, 0, time.UTC)
	b.now = morning

	// The fixture's admin seat is live, full and human, so the pass finds it —
	// and it holds no role granting deal reads. That is a configuration an
	// installation may legitimately have, not a fault: it must cost that seat
	// its brief and nothing else. Failing the job instead would deny the whole
	// team their morning over one seat, every hour, forever.
	if err := b.run(t); err != nil {
		t.Fatalf("the pass failed over a seat it could not assemble for: %v — one ungranted seat must not cost the workspace its morning", err)
	}
	if n := b.runsFor(t, b.AdminUser, morning); n != 0 {
		t.Fatalf("the ungranted seat got %d runs, want none", n)
	}
	for _, rep := range []ids.UUID{b.Rep1, b.Rep2, b.Rep3} {
		if n := b.runsFor(t, rep, morning); n != 1 {
			t.Fatalf("rep %s has %d runs, want the 1 the ungranted seat must not have cost them", rep, n)
		}
	}
}
