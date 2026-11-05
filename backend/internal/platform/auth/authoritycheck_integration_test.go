// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package auth

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// recordingAs is a recorder over a human holding perms.
func recordingAs(perms principal.Permissions) (context.Context, func() RecordedCall) {
	ctx := principal.WithActor(context.Background(), principal.Principal{Type: principal.PrincipalHuman, ID: "human:recorded", Permissions: perms})
	return RecordChecksForTest(ctx)
}

// Every refusal a gate answers is recorded as the check it refused, so a
// recorder can tell it from a 403 no gate made. A branch records nothing.
func TestEveryGateRefusalIsRecordedAndABranchIsNot(t *testing.T) {
	ctx, recorded := recordingAs(principal.Permissions{RowScope: principal.RowScopeOwn, Objects: map[string]principal.ObjectGrant{"deal": {Read: true}}})
	if Allows(ctx, "deal", principal.ActionUpdate) {
		t.Fatal("deal.update is admitted, and the seat holds only deal.read")
	}
	if got := recorded(); len(got.Checks)+len(got.Refused) > 0 {
		t.Errorf("a branch recorded %+v", got)
	}
	gates := []func() error{
		func() error { return Require(ctx, "deal", principal.ActionUpdate) },
		func() error { return RequireAdmin(ctx) },
		func() error { return RequireRowScope(ctx, principal.RowScopeTeam) },
		func() error { return RequireAny(ctx, "lead", principal.ActionCreate, principal.ActionUpdate) },
		func() error { return RequireAnyObject(ctx, principal.ActionDelete) },
		func() error {
			return RequireOneOf(ctx, Check{Object: "job_health", Actions: []principal.Action{principal.ActionRead}})
		},
	}
	for i, gate := range gates {
		if err := gate(); !errors.Is(err, apperrors.ErrPermissionDenied) {
			t.Errorf("gate %d admitted a seat it should refuse: %v", i, err)
		}
	}
	want := []string{"*.delete", "admin", "deal.update", "job_health.read", "lead.create|update", "scope:team"}
	if got := recorded().Refused; !slices.Equal(got, want) {
		t.Errorf("recorded refusals %v, want %v", got, want)
	}
}

// A store's own refusal of a record goes through RefuseRow, which answers the
// store's words and notes it.
func TestAStoresRowRefusalIsRecordedInItsOwnWords(t *testing.T) {
	ctx, recorded := recordingAs(principal.Permissions{})
	refusal := errors.New("recorded at a level you do not outrank: permission denied")
	if err := RefuseRow(ctx, "communication_suppression", refusal); !errors.Is(err, refusal) {
		t.Errorf("RefuseRow answered %v, want the store's refusal", err)
	}
	if got := recorded().Refused; !slices.Equal(got, []string{"row:communication_suppression"}) {
		t.Errorf("recorded refusals %v", got)
	}
}

// A team's week records the three grants its reach reads, refuses a seat it
// reaches no team for, and admits an overseer.
func TestATeamWeekRecordsTheGrantsItsReachReads(t *testing.T) {
	read := principal.ObjectGrant{Read: true}
	ctx, recorded := recordingAs(principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"deal": read}})
	if _, err := RequireTeamWeekReach(ctx); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat that leads and oversees nobody was answered %v", err)
	}
	got := recorded()
	if want := []string{"deal.read", "scope:team", "team_oversight.read,team_lead.read"}; !slices.Equal(got.Checks, want) {
		t.Errorf("recorded checks %v, want %v", got.Checks, want)
	}
	if !slices.Equal(got.Refused, []string{"team_oversight.read,team_lead.read"}) {
		t.Errorf("recorded refusals %v", got.Refused)
	}
	overseer, _ := recordingAs(principal.Permissions{RowScope: principal.RowScopeAll, Objects: map[string]principal.ObjectGrant{"deal": read, objTeamOversight: read}})
	if reach, err := RequireTeamWeekReach(overseer); err != nil || reach != ReachesEveryTeam {
		t.Errorf("an overseer reaches %v, %v", reach, err)
	}
}
