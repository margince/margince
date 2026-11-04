// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Only a call that reached somebody is contact: Reached and Put through move a
// contact's last-activity clock, and a call that left a voicemail, was told to
// call back later or reached nobody does not. Changing a call's outcome later
// refolds the clock. A call logged with no outcome counts, as every call did.
func TestOnlyACallThatReachedSomebodyMovesTheContactClock(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Quiet Quinn", nil)
	clock := func() *time.Time {
		t.Helper()
		var at *time.Time
		if err := integration.OwnerConn(t).QueryRow(context.Background(),
			`SELECT last_activity_at FROM contact WHERE id = $1`, contact).Scan(&at); err != nil {
			t.Fatal(err)
		}
		return at
	}
	logCall := func(outcome string, at time.Time) ids.ActivityID {
		t.Helper()
		in := activities.LogActivityInput{Kind: "call", Source: "manual", OccurredAt: &at,
			Subject: new("Ring Quinn"),
			Links:   []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}}
		if outcome != "" {
			in.CallOutcome = &outcome
		}
		row, _, err := e.Activities.LogActivity(e.Admin(), in)
		if err != nil {
			t.Fatal(err)
		}
		return ids.From[ids.ActivityKind](ids.UUID(row.Id))
	}
	base := clocktest.Now(t).Add(-48 * time.Hour).Truncate(time.Second)
	for i, outcome := range []string{"not_reached", "voicemail", "call_back_later"} {
		logCall(outcome, base.Add(time.Duration(i)*time.Minute))
	}
	if at := clock(); at != nil {
		t.Fatalf("calls that reached nobody moved the clock to %v", at)
	}
	reached := base.Add(time.Hour)
	call := logCall("reached", reached)
	if at := clock(); at == nil || !at.Equal(reached) {
		t.Fatalf("a reached call left the clock at %v, want %v", at, reached)
	}
	outcome := "voicemail"
	if _, err := e.Activities.UpdateActivity(e.Admin(), call, activities.UpdateActivityInput{CallOutcome: &outcome}); err != nil {
		t.Fatal(err)
	}
	if at := clock(); at != nil {
		t.Fatalf("a call changed to a voicemail still holds the clock at %v", at)
	}
	untold := base.Add(2 * time.Hour)
	logCall("", untold)
	if at := clock(); at == nil || !at.Equal(untold) {
		t.Fatalf("a call with no outcome left the clock at %v, want %v", at, untold)
	}
}

// Undo of a first pick puts the call back to "no outcome": the restore sends
// call_outcome as a clear, the activity store honours it, and the call counts
// as contact again.
func TestUndoingACallOutcomePutsTheCallBackToNoOutcome(t *testing.T) {
	e := integration.Setup(t)
	contact := e.SeedContact(t, "Undo Una", nil)
	at := clocktest.Now(t).Add(-time.Hour).Truncate(time.Second)
	call, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "call", Source: "manual", OccurredAt: &at,
		Subject: new("Ring Una"), Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}}})
	if err != nil {
		t.Fatal(err)
	}
	id := ids.UUID(call.Id)
	outcome := "voicemail"
	if _, err := e.Activities.UpdateActivity(e.Admin(), ids.From[ids.ActivityKind](id), activities.UpdateActivityInput{CallOutcome: &outcome}); err != nil {
		t.Fatal(err)
	}
	var auditID ids.UUID
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `SELECT id FROM audit_log
		WHERE entity_type='activity' AND entity_id=$1 AND action='update' AND after->>'call_outcome'='voicemail'`, id).Scan(&auditID); err != nil {
		t.Fatalf("the pick wrote no audit row: %v", err)
	}
	seam := NewRestoreSeam(e.Pool, NewProvider(e.Pool), nil)
	if _, err := seam.Restore(e.Admin(), "activity", id, auditID, currentVersion(t, e, "activity", id)); err != nil {
		t.Fatalf("undoing the outcome: %v", err)
	}
	var stored *string
	var clock *time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(),
		`SELECT a.call_outcome, c.last_activity_at FROM activity a, contact c WHERE a.id=$1 AND c.id=$2`, id, contact).Scan(&stored, &clock); err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		t.Errorf("Undo left the outcome %q", *stored)
	}
	if clock == nil || !clock.Equal(at) {
		t.Errorf("after Undo the contact clock reads %v, want the call at %v", clock, at)
	}
}
