// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The batched deal-figures read, against a real database.
//
// Its whole claim is that it answers under the reader's own row scope, and that
// is SQL: a unit test with hand-built rows cannot fail it. The admission is
// asserted as hard as the refusal, because a read that answered nobody would
// pass a refusal-only suite while leaving every card blank.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedFiguresDeal writes one deal owned by the given rep, with the figures a
// card states.
func seedFiguresDeal(t *testing.T, owner ids.UUID, amount int64) ids.UUID {
	t.Helper()
	return seedFiguresDealClosing(t, owner, amount, time.Date(2026, 8, 28, 0, 0, 0, 0, time.UTC))
}

// seedFiguresDealClosing writes one deal with the given expected close date —
// the workspace's own timezone setting is what Figures reads it against
// (UTC, the fresh-installation default this harness never overrides).
func seedFiguresDealClosing(t *testing.T, owner ids.UUID, amount int64, closes time.Time) ids.UUID {
	t.Helper()
	conn := OwnerConn(t)
	pipeline := SeedIDRow(t, conn, `INSERT INTO pipeline (id, name, is_default, position)
		VALUES ($1, 'Sales', true, 0)`)
	stage := SeedIDRow(t, conn, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	return SeedIDRow(t, conn, `INSERT INTO deal
		(id, owner_id, name, pipeline_id, stage_id, amount_minor, currency, expected_close_date,
		 source, captured_by)
		VALUES ($1, $2, 'Northstar renewal', $3, $4, $5, 'EUR', $6, 'manual', 'human:x')`,
		owner, pipeline, stage, amount, closes)
}

// A deal the reader may see comes back with the figures a card states.
func TestDealFiguresAnswerADealTheReaderMaySee(t *testing.T) {
	e := Setup(t)
	const amount = int64(160_100_00)
	dealID := seedFiguresDeal(t, e.Rep1, amount)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("a deal the reader owns did not come back at all")
	}
	if got.AmountMinor == nil || *got.AmountMinor != amount {
		t.Fatalf("the deal came back worth %v, wanted %d", got.AmountMinor, amount)
	}
	if got.Currency != "EUR" {
		t.Fatalf("the deal came back in %q — a figure whose units are unnamed is dropped", got.Currency)
	}
	if got.ExpectedCloseDate == nil {
		t.Fatal("the deal came back with no close date, which is half of why a card is urgent")
	}
	if got.OwnerID != e.Rep1 {
		t.Fatalf("the deal came back owned by %v, wanted %v", got.OwnerID, e.Rep1)
	}
}

// An archived deal is not answered. A card naming a deal nobody works any more
// would send a rep at a closed conversation.
func TestDealFiguresWithholdAnArchivedDeal(t *testing.T) {
	e := Setup(t)
	dealID := seedFiguresDeal(t, e.Rep1, 50_000_00)
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE deal SET archived_at = now() WHERE id = $1`, dealID); err != nil {
		t.Fatalf("archiving the deal: %v", err)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("an archived deal failed the read: %v", err)
	}

	if _, found := figures[dealID]; found {
		t.Fatal("an archived deal came back")
	}
}

// Asking about nothing answers nothing. A page whose rows all carry their own
// figures must not pay for a read.
func TestDealFiguresAnswerNothingForNoIDs(t *testing.T) {
	e := Setup(t)

	figures, err := e.Deals.Figures(e.Admin(), nil)
	if err != nil {
		t.Fatalf("asking about no deals: %v", err)
	}
	if len(figures) != 0 {
		t.Fatalf("asking about no deals answered %d", len(figures))
	}
}

// The role mask reaches this read too.
//
// Row scope decides which deals answer; the field mask decides which of their
// columns do. A rep whose role hides another team's amount everywhere else must
// not read it off the Worklist, which is the one surface that used to get its
// figures from a door with no mask on it.
func TestDealFiguresApplyTheRolesFieldMask(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	mine := e.SeedDeal(t, "Mine", pipeline, open, &e.Rep1)
	theirs := e.SeedDeal(t, "Theirs", pipeline, open, &e.Rep3)
	const amount = int64(250000)
	for _, id := range []ids.UUID{mine, theirs} {
		e.WsExec(t, `UPDATE deal SET amount_minor = $2, currency = 'EUR' WHERE id = $1`, id, amount)
	}
	perms := activityLifecyclePerms
	perms.Objects = map[string]principal.ObjectGrant{
		"deal": {Read: true, Update: true}, "pipeline": {Read: true},
		// Figures reads the installation's timezone to state each deal's own
		// overdue verdict — installation_settings mirrors 0191's real seed,
		// readable by every seeded role, so a narrow test fixture needs it
		// stated explicitly the way dozens of others in this package already do.
		"installation_settings": {Read: true},
	}
	perms.FieldMasks = []principal.FieldMask{
		{Object: "deal", Field: "amount_minor", Condition: principal.MaskOutsideWriteAuthority},
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{mine, theirs})
	if err != nil {
		t.Fatalf("reading figures under a mask: %v", err)
	}

	// The admission half: the mask must not swallow the rep's own money.
	own, ok := figures[mine]
	if !ok {
		t.Fatal("the rep's own deal did not come back at all")
	}
	if own.AmountMinor == nil || *own.AmountMinor != amount || own.Currency != "EUR" {
		t.Fatalf("the rep's own deal came back worth %v %q, wanted %d EUR",
			own.AmountMinor, own.Currency, amount)
	}
	// And the refusal: another team's money is withheld, currency with it.
	other, ok := figures[theirs]
	if !ok {
		t.Fatal("another team's deal vanished — a deal a rep may READ should still name itself")
	}
	if other.AmountMinor != nil {
		t.Fatalf("another team's amount reached the Worklist as %d", *other.AmountMinor)
	}
	if other.Currency != "" {
		t.Fatalf("another team's currency reached the Worklist as %q — a currency alone says the deal is priced and in what units", other.Currency)
	}
}

// A close date already behind today's calendar date, in the
// workspace's own zone (UTC — this harness's fresh-installation default),
// answers overdue — the same verdict deals.CloseIsOverdue gives the at-risk
// lane over the identical deal, so the Worklist's two rows for one deal state
// one verdict about whether it is late.
func TestDealFiguresFlagsAPastCloseDateAsOverdue(t *testing.T) {
	e := Setup(t)
	yesterday := time.Now().UTC().AddDate(0, 0, -1)
	dealID := seedFiguresDealClosing(t, e.Rep1, 50_000_00, yesterday)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("the deal did not come back at all")
	}
	if !got.CloseOverdue {
		t.Fatal("a close date already behind today's calendar date came back not overdue")
	}
}

// A close date that has not yet arrived — today's or later — is not overdue.
// Today itself is the boundary this asserts: CloseIsOverdue is a calendar-date
// comparison, and a deal due today is due today, not late.
func TestDealFiguresDoesNotFlagATodayOrFutureCloseDateAsOverdue(t *testing.T) {
	e := Setup(t)
	// The DB's own "today", not Go's: Figures compares against Postgres's
	// now() in the installation's zone, and seeding from the wall clock this
	// process reads would race a UTC-midnight rollover between the seed and
	// the query.
	var today time.Time
	if err := OwnerConn(t).QueryRow(context.Background(),
		`SELECT (timezone('UTC', now()))::date`).Scan(&today); err != nil {
		t.Fatalf("reading the database's own today: %v", err)
	}
	dealID := seedFiguresDealClosing(t, e.Rep1, 50_000_00, today)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("the deal did not come back at all")
	}
	if got.CloseOverdue {
		t.Fatal("a deal due TODAY came back overdue")
	}
}
