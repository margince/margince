// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// A deal whose newest activity names only a date is counted quiet from that
// date. At UTC+14 its noon-UTC anchor is already the next day, so counting
// from the stored instant read the deal a day less quiet than it is.
func TestATeamsStalledDealCountsADateOnlyActivityFromItsDate(t *testing.T) {
	e := setupTeamWeekly(t)
	writeRepWeek(t, e, e.Rep1)
	engine := weekly.NewEngine(e.Pool, identity.NewService(e.Pool)).WithAttention(compose.WeeklyAttention{})
	if _, _, err := engine.AssembleTeamFor(e.leadCtx, e.Team1, "Team One", members(e), teamClock); err != nil {
		t.Fatal(err)
	}
	kiritimati, err := time.LoadLocation("Pacific/Kiritimati")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = to_jsonb('Pacific/Kiritimati'::text) WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	pipeline, open := defaultOpenStages(t, e)
	quiet := e.SeedDeal(t, "Visited, then nothing", pipeline, ids.From[ids.StageKind](open[0]), &e.Rep2)
	e.WsExec(t, `UPDATE deal SET created_at = now() - interval '120 days' WHERE id = $1`, quiet)

	utcToday := time.Now().UTC()
	visited := time.Date(utcToday.Year(), utcToday.Month(), utcToday.Day()-40, 12, 0, 0, 0, time.UTC)
	subject := "Site visit"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Source: provenance.RecordSourceManual, Subject: &subject,
		OccurredAt: &visited, OccurredPrecision: activities.PrecisionDay,
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: quiet}},
	}); err != nil {
		t.Fatalf("logging the visit: %v", err)
	}

	read, err := engine.LatestTeamReview(e.leadCtx, e.Team1, nil)
	if err != nil {
		t.Fatal(err)
	}
	local := time.Now().In(kiritimati)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
	want := int(today.Sub(time.Date(visited.Year(), visited.Month(), visited.Day(), 0, 0, 0, 0, time.UTC)).Hours() / 24)
	stale := read.Attention.StaleDeals
	if len(stale) != 1 || stale[0].DealID != quiet || stale[0].DaysQuiet != want {
		t.Errorf("stalled deals %+v, want only %v at %d days quiet from the visit's date", stale, quiet, want)
	}
}
