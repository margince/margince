// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// asMember is one member reading activities, as a browser tab of theirs does.
func asMember(e *sendEnv, member ids.UUID) context.Context {
	ctx := principal.WithCorrelationID(principal.WithWorkspaceID(context.Background(), e.ws), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + member.String(), UserID: member,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects:  map[string]principal.ObjectGrant{"activity": {Read: true, Create: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
}

// A member signed in on two browsers sees a due reminder once. The first tab
// to claim it shows it; a moved reminder can be claimed again.
func TestADueReminderIsClaimedOncePerMemberAndAgainOnceMoved(t *testing.T) {
	e := setupSend(t)
	now := clocktest.Now(t)
	store := pinStore(e).WithClock(func() time.Time { return now })
	rep := asMember(e, e.rep)
	due := now.Add(-time.Minute).Truncate(time.Microsecond)
	assignee := ids.From[ids.UserKind](e.rep)
	created, _, err := store.LogActivity(rep, LogActivityInput{
		Kind: "task", Subject: new("Call back"), Source: "manual", RemindAt: &due, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	task := ids.From[ids.ActivityKind](ids.UUID(created.Id))
	claim := func(ctx context.Context, at time.Time) bool {
		t.Helper()
		claimed, err := store.ClaimReminderDelivery(ctx, task, at)
		if err != nil {
			t.Fatal(err)
		}
		return claimed
	}

	if !claim(rep, due) {
		t.Fatal("the first tab's claim was refused")
	}
	if claim(rep, due) {
		t.Error("a second tab claimed a reminder the first already showed")
	}
	if claim(asMember(e, e.other), due) {
		t.Error("a colleague claimed a reminder on a task that is not theirs")
	}

	moved := due.Add(-time.Second)
	if _, err := store.UpdateActivity(rep, task, UpdateActivityInput{RemindAt: &moved}); err != nil {
		t.Fatal(err)
	}
	if claim(rep, due) {
		t.Error("the old reminder instant was claimed after the reminder moved")
	}
	if !claim(rep, moved) {
		t.Error("the moved reminder could not be claimed again")
	}
}
