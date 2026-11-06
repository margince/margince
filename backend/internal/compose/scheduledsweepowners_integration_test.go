// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The overnight sweep runs once per member who granted it. Each run's slipping
// list is that member's own deals, and a run writes at most one note per deal.
// So two members' runs leave one note on each of their deals. The deal of a
// member who never granted the sweep gets none.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func TestTwoMembersSweepsLeaveOneNoteOnEachOfTheirDeals(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	quietSince := clocktest.Now(t).AddDate(0, 0, -90)
	owned := map[ids.UUID]ids.UUID{}
	for _, owner := range []ids.UUID{e.Rep1, e.Rep2, e.Rep3} {
		deal := e.SeedDeal(t, "Quiet deal", pipeline, stage, &owner)
		e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, deal, quietSince)
		owned[owner] = deal
	}
	lister := slippingLister(e.Pool)
	if every, err := lister(e.Admin(), runner.SweepQuietDays, nil); err != nil || len(every) != 3 {
		t.Fatalf("outside a run the list holds %d deals (err %v), want all 3", len(every), err)
	}

	// Rep3 never granted the sweep, so no run acts for them.
	for _, member := range []ids.UUID{e.Rep1, e.Rep2} {
		run := principal.WithAgentRunID(e.AgentFor(t, member, nil, integration.AdminPerms), ids.NewV7())
		listed, err := lister(run, runner.SweepQuietDays, nil)
		if err != nil {
			t.Fatal(err)
		}
		if len(listed) != 1 || listed[0].DealID != owned[member] {
			t.Fatalf("the run for %s lists %+v, want only their own deal", member, listed)
		}
		// A model asking twice for the same deal writes one note.
		for range 2 {
			if err := riskNote(run, t, e, listed[0].DealID); err != nil {
				t.Fatalf("logging the risk note: %v", err)
			}
		}
	}

	for owner, deal := range owned {
		want := 1
		if owner == e.Rep3 {
			want = 0
		}
		got := e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
			WHERE a.kind = 'note' AND l.deal_id = $1`, deal)
		if got != want {
			t.Errorf("the deal %s owns carries %d risk notes, want %d", owner, got, want)
		}
	}
}

// riskNote logs a risk note on deals through the activities provider, as the
// sweep's log_activity call does.
func riskNote(run context.Context, t *testing.T, e *integration.Env, deals ...ids.UUID) error {
	t.Helper()
	links := make([]map[string]any, 0, len(deals))
	for _, deal := range deals {
		links = append(links, map[string]any{"entity_type": "deal", "entity_id": deal.String()})
	}
	_, err := activities.NewProvider(e.DB()).Create(run, datasource.CreateInput{
		EntityType: datasource.EntityActivity, Source: "manual",
		Fields: map[string]any{
			"kind": "note", "subject": "Risk assessment", "body": "Quiet for 90 days.",
			"occurred_at": clocktest.Now(t).Add(-time.Minute).Format(time.RFC3339), "links": links,
		},
	})
	return err
}

// sweepFixture is one open deal for each of three members, quiet for the
// same number of days.
type sweepFixture struct {
	e     *integration.Env
	owned map[ids.UUID]ids.UUID
}

func newSweepFixture(t *testing.T, quietDays int) sweepFixture {
	t.Helper()
	f := sweepFixture{e: integration.Setup(t), owned: map[ids.UUID]ids.UUID{}}
	pipeline, stage, _ := integration.DealFixture(t, f.e)
	quietSince := clocktest.Now(t).AddDate(0, 0, -quietDays)
	for _, owner := range []ids.UUID{f.e.Rep1, f.e.Rep2, f.e.Rep3} {
		deal := f.e.SeedDeal(t, "Quiet deal", pipeline, stage, &owner)
		f.e.WsExec(t, `UPDATE deal SET created_at = $2, last_activity_at = $2 WHERE id = $1`, deal, quietSince)
		f.owned[owner] = deal
	}
	return f
}

func (f sweepFixture) run(t *testing.T, member ids.UUID) context.Context {
	t.Helper()
	return principal.WithAgentRunID(f.e.AgentFor(t, member, nil, integration.AdminPerms), ids.NewV7())
}

func (f sweepFixture) notesOn(t *testing.T, deal ids.UUID) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE a.kind = 'note' AND l.deal_id = $1`, deal)
}

// The owner is checked when the note is written, not only when the deal was
// listed. A colleague's deal is refused, and so is a deal reassigned after
// the run listed it.
func TestAScheduledRunMayNotFileUnderADealItsMemberDoesNotOwn(t *testing.T) {
	f := newSweepFixture(t, 90)
	run := f.run(t, f.e.Rep1)

	colleagues := f.owned[f.e.Rep2]
	if _, refused := errors.AsType[*activities.ScheduledRunForeignDealError](riskNote(run, t, f.e, colleagues)); !refused {
		t.Errorf("a run filed under a colleague's deal, want ScheduledRunForeignDealError")
	}
	listed, err := slippingLister(f.e.Pool)(run, runner.SweepQuietDays, nil)
	if err != nil || len(listed) != 1 {
		t.Fatalf("the run lists %d deals (err %v), want its own one", len(listed), err)
	}
	f.e.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, listed[0].DealID, f.e.Rep2)
	if _, refused := errors.AsType[*activities.ScheduledRunForeignDealError](riskNote(run, t, f.e, listed[0].DealID)); !refused {
		t.Errorf("a run filed under a deal reassigned after it was listed, want ScheduledRunForeignDealError")
	}
	for _, deal := range []ids.UUID{colleagues, listed[0].DealID} {
		if n := f.notesOn(t, deal); n != 0 {
			t.Errorf("a refused note left %d notes on the deal", n)
		}
	}
}

// One note per deal and member a day. A note on two deals keeps the deal that
// has none yet. A new passport for the same member does not lift the limit.
func TestTheNoteLimitIsPerDealAndPerMember(t *testing.T) {
	f := newSweepFixture(t, 90)
	first, second := f.owned[f.e.Rep1], f.owned[f.e.Rep2]
	f.e.WsExec(t, `UPDATE deal SET owner_id = $2 WHERE id = $1`, second, f.e.Rep1)

	if err := riskNote(f.run(t, f.e.Rep1), t, f.e, first); err != nil {
		t.Fatal(err)
	}
	if err := riskNote(f.run(t, f.e.Rep1), t, f.e, first, second); err != nil {
		t.Fatal(err)
	}
	if got := [2]int{f.notesOn(t, first), f.notesOn(t, second)}; got != [2]int{1, 1} {
		t.Fatalf("notes on the two deals = %v, want one each", got)
	}

	passport := f.e.SeedPassportFor(t, integration.OwnerConn(t), "rotated", f.e.Rep1)
	rotated := principal.WithAgentRunID(
		f.e.AgentOnPassport(t, f.e.Rep1, nil, integration.AdminPerms, ids.From[ids.PassportKind](passport)), ids.NewV7())
	if err := riskNote(rotated, t, f.e, second); err != nil {
		t.Fatal(err)
	}
	if n := f.notesOn(t, second); n != 1 {
		t.Errorf("a new passport for the same member wrote a second note (%d) on the deal", n)
	}
}

// The sweep's preflight and its tool read one window, so a deal quiet for 30
// days is on the run's list either way.
func TestADealQuietForThirtyDaysIsOnTheRunsList(t *testing.T) {
	f := newSweepFixture(t, 30)
	listed, err := slippingLister(f.e.Pool)(f.run(t, f.e.Rep1), runner.SweepQuietDays, nil)
	if err != nil || len(listed) != 1 || listed[0].DealID != f.owned[f.e.Rep1] {
		t.Fatalf("the run lists %+v (err %v), want the deal quiet for 30 days", listed, err)
	}
}
