// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/projects"
)

// A project whose newest activity names only a date has gone quiet once that
// many dates have passed in the installation's calendar. At UTC+14 the noon-UTC
// anchor is already the next day, so measuring from it raised the project a day late.
func TestAProjectWithADateOnlyActivityGoesQuietByItsDate(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	if _, err := OwnerConn(t).Exec(context.Background(),
		`UPDATE setting SET value = to_jsonb('Pacific/Kiritimati'::text) WHERE key = 'installation.timezone'`); err != nil {
		t.Fatal(err)
	}
	utcToday := time.Now().UTC()
	visited := time.Date(utcToday.Year(), utcToday.Month(), utcToday.Day()-60, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Day Client", nil)
	rollout := seedProject(admin, t, e, "Rollout", company, &e.Rep1)
	advanceProject(admin, t, e, rollout.ID, projects.PhaseDelivering)
	subject := "Site visit"
	if _, _, err := e.Activities.LogActivity(admin, activities.LogActivityInput{
		Kind: "note", Subject: &subject, OccurredAt: &visited, OccurredPrecision: activities.PrecisionDay, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "project", EntityID: rollout.ID.UUID}},
	}); err != nil {
		t.Fatalf("logging the visit: %v", err)
	}

	// At 11:00 UTC on the 29th day after the visit it is already the 30th in
	// Kiritimati, while fewer than 30 times 24 hours have passed since noon UTC.
	quietDay := visited.AddDate(0, 0, projects.DefaultProjectQuietDays-1).Add(-time.Hour)
	if pass := quietProjectPass(t, e, quietDay); pass.Considered != 1 {
		t.Errorf("on the date the quiet period ends, %d project(s) were considered, want the rollout", pass.Considered)
	}
	if pass := quietProjectPass(t, e, quietDay.Add(-24*time.Hour)); pass.Considered != 0 {
		t.Errorf("a day before that, %d project(s) were considered, want none", pass.Considered)
	}
}
