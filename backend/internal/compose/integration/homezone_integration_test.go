// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A Hanoi rep on a Berlin installation, read through the real Worklist
// assembly. The two cities are on different dates at 18:30 UTC on 15 June.
// It is 20:30 on the 15th in Berlin and 01:30 on the 16th in Hanoi.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var hanoiBerlinMoment = time.Date(2026, 6, 15, 18, 30, 0, 0, time.UTC)

func hanoiBerlinSetup(t *testing.T) *Env {
	t.Helper()
	e := Setup(t)
	e.WsExec(t, `UPDATE setting SET value = '"Europe/Berlin"'::jsonb WHERE key = 'installation.timezone'`)
	e.WsExec(t, `UPDATE app_user SET timezone = 'Asia/Ho_Chi_Minh' WHERE id = $1`, e.Rep1)
	e.WsExec(t, `UPDATE app_user SET timezone = NULL WHERE id = $1`, e.Rep2)
	return e
}

func ownWorklist(t *testing.T, e *Env, user ids.UUID) map[string]struct {
	quietDays *int
	overdue   *bool
} {
	t.Helper()
	feed := compose.NewAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return hanoiBerlinMoment })
	day, err := feed.Worklist(e.As(user, []ids.UUID{e.Team1}, AdminPerms), "mine", "all", ids.Nil, 100, "")
	if err != nil {
		t.Fatal(err)
	}
	rows := map[string]struct {
		quietDays *int
		overdue   *bool
	}{}
	for _, row := range day.Queue {
		if row.Title == nil {
			continue
		}
		var quiet *int
		if row.Deal != nil {
			quiet = row.Deal.QuietDays
		}
		rows[string(row.Source)+"/"+*row.Title] = struct {
			quietDays *int
			overdue   *bool
		}{quiet, row.Overdue}
	}
	return rows
}

// A plan commitment dated the 16th is due today in Hanoi. In Berlin it is due
// tomorrow. So it is on the Hanoi rep's own list and not on a Berlin rep's.
func TestAPlanCommitmentDueTodayInHanoiShowsOnTheHanoiRepsOwnHome(t *testing.T) {
	e := hanoiBerlinSetup(t)
	store := compose.WeeklyPlanStore(e.Pool)
	date := time.Date(2026, 6, 16, 0, 0, 0, 0, time.UTC)
	for _, rep := range []ids.UUID{e.Rep1, e.Rep2} {
		ctx := e.As(rep, []ids.UUID{e.Team1}, AdminPerms)
		if _, err := store.StartWeek(ctx, hanoiBerlinMoment); err != nil {
			t.Fatal(err)
		}
		if _, err := store.AddCommitment(ctx, hanoiBerlinMoment, weeklyplan.NewCommitment{Label: "Call the buyer", DueOn: &date}); err != nil {
			t.Fatal(err)
		}
	}
	const row = "weekly_commitment/Call the buyer"
	if _, shown := ownWorklist(t, e, e.Rep1)[row]; !shown {
		t.Error("the Hanoi rep's own Worklist lacks the commitment due today in Hanoi")
	}
	if _, shown := ownWorklist(t, e, e.Rep2)[row]; shown {
		t.Error("a rep on the installation's Berlin day sees a commitment that is due tomorrow for them")
	}
}

// A deal closing on the 15th is overdue in Hanoi, so the Hanoi rep's at-risk
// lane admits it. Its quiet days count on the same Hanoi day.
func TestAnAtRiskDealsQuietDaysCountOnTheDayItWasAdmittedOn(t *testing.T) {
	e := hanoiBerlinSetup(t)
	pipeline, open, _ := DealFixture(t, e)
	closeOn := time.Date(2026, 6, 15, 0, 0, 0, 0, time.UTC)
	owner := ids.From[ids.UserKind](e.Rep1)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Hanoi deal", PipelineID: pipeline, StageID: open, OwnerID: &owner, OwnerExact: true, ExpectedClose: &closeOn,
	})
	if err != nil {
		t.Fatal(err)
	}
	// Created at 05:30 on the 15th in Hanoi and 00:30 on the 15th in Berlin: one
	// quiet day on the Hanoi calendar, none on Berlin's.
	e.WsExec(t, `UPDATE deal SET created_at = $2 WHERE id = $1`, ids.UUID(deal.Id), time.Date(2026, 6, 14, 22, 30, 0, 0, time.UTC))

	got, shown := ownWorklist(t, e, e.Rep1)["deal_at_risk/Hanoi deal"]
	if !shown {
		t.Fatal("the deal overdue in Hanoi is missing from the Hanoi rep's own Worklist")
	}
	if got.overdue == nil || !*got.overdue {
		t.Errorf("overdue = %v, want true on the Hanoi day", got.overdue)
	}
	if got.quietDays == nil || *got.quietDays != 1 {
		t.Errorf("quiet days = %v, want 1 on the same Hanoi calendar", got.quietDays)
	}
}
