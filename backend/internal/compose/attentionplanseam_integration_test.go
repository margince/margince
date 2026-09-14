// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/weeklyplan"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestPlanAgendaUsesStoredDatesAndCompletion(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	store := weeklyPlanStore(e.Pool)
	now := time.Date(2026, 6, 10, 12, 0, 0, 0, time.UTC)
	if _, err := store.StartWeek(ctx, now); err != nil {
		t.Fatal(err)
	}
	dates := []*time.Time{nil}
	for _, day := range []int{9, 10, 11} {
		date := time.Date(2026, 6, day, 0, 0, 0, 0, time.UTC)
		dates = append(dates, &date)
	}
	var today ids.UUID
	for index, date := range dates {
		row, err := store.AddCommitment(ctx, now, weeklyplan.NewCommitment{Label: "Call the buyer", DueOn: date})
		if err != nil {
			t.Fatal(err)
		}
		if index == 2 {
			today = row.ID
		}
	}
	seam := attentionWeeklyPlan{store: store, pool: e.Pool}
	rows, err := seam.DuePlan(ctx, ids.UUID{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 {
		t.Fatalf("wanted yesterday and today, got %+v", rows)
	}
	for _, row := range rows {
		if row.ID == today && row.DueAt.Before(now) {
			t.Fatal("today's date-only promise is already overdue")
		}
	}
	if err := store.SetState(ctx, today, weeklyplan.StateDone); err != nil {
		t.Fatal(err)
	}
	rows, err = seam.DuePlan(ctx, ids.UUID{}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].ID == today {
		t.Fatalf("completed commitment remained: %+v", rows)
	}
}

func TestNamedTeamRosterDoesNotIncludeAnotherTeam(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.AdminUser, nil, integration.AdminPerms)
	seam := newTeammatesSeam(e.Pool)
	members, cut, err := seam.LiveMembersOfTeam(ctx, e.Team1)
	if err != nil {
		t.Fatal(err)
	}
	if cut || len(members) == 0 {
		t.Fatalf("unexpected roster size %d, cut=%v", len(members), cut)
	}
	found := false
	for _, member := range members {
		if member.UserID == e.Rep3 {
			t.Fatal("other team member leaked into named team")
		}
		found = found || member.UserID == e.Rep1
	}
	if !found {
		t.Fatal("selected team's member was omitted")
	}
}

func TestNamedTeamRosterRequiresTheCallersLiveTeamScope(t *testing.T) {
	e := integration.Setup(t)
	seam := newTeammatesSeam(e.Pool)
	team := integration.RepPerms
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, team)
	if members, _, err := seam.LiveMembersOfTeam(ctx, e.Team1); err != nil || len(members) == 0 {
		t.Fatalf("own live team: %d members, %v", len(members), err)
	}
	if _, _, err := seam.LiveMembersOfTeam(ctx, e.Team2); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("other team: %v", err)
	}
	own := team
	own.RowScope = principal.RowScopeOwn
	if _, _, err := seam.LiveMembersOfTeam(e.As(e.Rep1, []ids.UUID{e.Team1}, own), e.Team1); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("own-only scope: %v", err)
	}
}

func TestPlanAgendaUsesTheInstallationCalendarAcrossDST(t *testing.T) {
	e := integration.Setup(t)
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	zone := "America/New_York"
	if _, err := identity.NewInstallationSettings(e.DB(), NewSettingsStore(e.Pool)).UpdateInstallation(ctx, identity.InstallationPatch{Timezone: &zone}); err != nil {
		t.Fatal(err)
	}
	store := weeklyPlanStore(e.Pool)
	now := time.Date(2026, 3, 8, 6, 30, 0, 0, time.UTC)
	if _, err := store.StartWeek(ctx, now); err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	if _, err := store.AddCommitment(ctx, now, weeklyplan.NewCommitment{Label: "Confirm the next step", DueOn: &date}); err != nil {
		t.Fatal(err)
	}
	rows, err := (attentionWeeklyPlan{store: store, pool: e.Pool}).DuePlan(ctx, ids.UUID{}, now)
	if err != nil {
		t.Fatal(err)
	}
	expected := time.Date(2026, 3, 9, 3, 59, 59, 0, time.UTC)
	if len(rows) != 1 || !rows[0].DueAt.Equal(expected) {
		t.Fatalf("DST local end of day: %+v; expected %v", rows, expected)
	}
}
