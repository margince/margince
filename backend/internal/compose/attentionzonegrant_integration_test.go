// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A member without installation_settings.read still gets the installation's
// day on the Worklist: which day it is needs no settings grant.
func TestTheWorklistDayBoundaryNeedsNoSettingsGrant(t *testing.T) {
	t.Parallel()
	e := integration.Setup(t)
	zoneName := "Pacific/Auckland"
	zone, err := time.LoadLocation(zoneName)
	if err != nil {
		t.Fatalf("loading %s: %v", zoneName, err)
	}
	if _, err := NewInstallationSettingsFor(e.Pool).UpdateInstallation(e.Admin(), identity.InstallationPatch{Timezone: &zoneName}); err != nil {
		t.Fatalf("choosing the installation zone: %v", err)
	}
	read := principal.ObjectGrant{Read: true}
	rep := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		Objects: map[string]principal.ObjectGrant{
			"activity": {Read: true, Create: true, Update: true}, "automation": read, "company": read, "contact": read,
			"deal": read, "forecast": read, "introduction": read, "lead": read, "project": read,
			"relationship": read, "weekly_plan": read,
		},
		RowScope: principal.RowScopeAll,
	})

	// Two hours before Auckland's midnight, a UTC day still has over ten hours
	// to run, so only the installation's zone puts the second task tomorrow.
	local := clocktest.Now(t).In(zone)
	endOfDay := time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, zone)
	now := endOfDay.Add(-2 * time.Hour)
	for subject, due := range map[string]time.Time{
		"Due a second before the installation's midnight": endOfDay.Add(-time.Second),
		"Due at the installation's midnight":              endOfDay,
	} {
		if _, _, err := e.Activities.LogActivity(rep, activities.LogActivityInput{Kind: "task", Subject: &subject, DueAt: &due, Source: "manual"}); err != nil {
			t.Fatalf("logging task %q: %v", subject, err)
		}
	}

	feed := newAttentionService(e.Pool, approvals.NewService(e.DB()), func() time.Time { return now })
	day, err := feed.Assemble(rep)
	if err != nil {
		t.Fatalf("a member without installation_settings.read could not read the day: %v", err)
	}
	groups := map[string]string{}
	for _, item := range day.Planned {
		if item.Title != nil && item.DueGroup != nil {
			groups[*item.Title] = string(*item.DueGroup)
		}
	}
	if got := groups["Due a second before the installation's midnight"]; got != "today" {
		t.Errorf("a task due a second before the installation's midnight is grouped %q, want today", got)
	}
	if got := groups["Due at the installation's midnight"]; got != "tomorrow" {
		t.Errorf("a task due at the installation's midnight is grouped %q, want tomorrow", got)
	}
}
