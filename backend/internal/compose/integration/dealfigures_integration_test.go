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
//
// closes is bound as a plain calendar-date string, not the time.Time itself:
// a date literal parses to the exact day named, with no session TimeZone GUC
// in between, so a caller stating "today" in UTC gets that same day in the
// column whatever the connection's zone happens to be.
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
		VALUES ($1, $2, 'Northstar renewal', $3, $4, $5, 'EUR', $6::date, 'manual', 'human:x')`,
		owner, pipeline, stage, amount, closes.Format(time.DateOnly))
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
//
// The seed and Figures' own read of "now" must agree on what day it is: a
// wall-clock instant seeded here and re-read by Figures moments later could
// straddle UTC midnight between the two calls, which would make the seeded
// date read as YESTERDAY and fail this test on a defect it does not have. One
// pinned instant closes that gap.
func TestDealFiguresDoesNotFlagATodayOrFutureCloseDateAsOverdue(t *testing.T) {
	e := Setup(t)
	now := time.Now().UTC()
	e.Deals.WithClock(func() time.Time { return now })
	dealID := seedFiguresDealClosing(t, e.Rep1, 50_000_00, now)
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

// The figures carry the win probability recorded on the deal's stage.
//
// It reaches the Worklist card beside the deal's own money, so a reader sees
// the stage's odds without reading the stage. Asserted against the DATABASE
// rather than the projection alone, because the column lives on another table
// and the only thing that proves the join reaches it is a real one — a wrong
// table name compiles perfectly and fails on the first query.
func TestDealFiguresCarryTheStagesWinProbability(t *testing.T) {
	e := Setup(t)
	dealID := seedFiguresDeal(t, e.Rep1, 160_100_00)
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("a deal the reader owns did not come back at all")
	}
	// The stage seedFiguresDealClosing writes records 10.
	if got.StageWinProbability == nil {
		t.Fatal("the deal came back with no win probability, though its stage records one — the " +
			"card then states nothing and a reader goes to the stage for it")
	}
	if *got.StageWinProbability != 10 {
		t.Errorf("win probability came back %d, wanted the stage's 10", *got.StageWinProbability)
	}
	if got.AmountMinor == nil || *got.AmountMinor != 160_100_00 {
		t.Errorf("amount_minor = %v, want 16010000 unweighted — reading the probability must not "+
			"start applying it", got.AmountMinor)
	}
}

// A stage scored zero reaches the reader as zero, not as absent.
//
// This is the case the pointer exists for. `stage.win_probability` is NOT NULL,
// so every deal arrives with a score; what the type has to keep apart is a
// stage nobody rates highly from a fact nobody stated. Collapsing them would
// tell a reader "unscored" about a deal the pipeline scores at 0.
func TestADealOnAStageScoredZeroCarriesZeroRatherThanNothing(t *testing.T) {
	e := Setup(t)
	dealID := seedFiguresDeal(t, e.Rep1, 50_000_00)
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE stage SET win_probability = 0
		  WHERE id = (SELECT stage_id FROM deal WHERE id = $1)`, dealID); err != nil {
		t.Fatalf("scoring the stage zero: %v", err)
	}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, RepPerms)

	figures, err := e.Deals.Figures(rep, []ids.UUID{dealID})
	if err != nil {
		t.Fatalf("reading the deal's figures: %v", err)
	}

	got, ok := figures[dealID]
	if !ok {
		t.Fatal("the deal did not come back at all")
	}
	if got.StageWinProbability == nil {
		t.Fatal("a stage scored 0 came back as no score at all — a reader is told nobody rated " +
			"this deal when the pipeline rates it at zero")
	}
	if *got.StageWinProbability != 0 {
		t.Errorf("win probability came back %d, wanted the stage's 0", *got.StageWinProbability)
	}
}
