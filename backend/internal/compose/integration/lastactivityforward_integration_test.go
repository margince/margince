// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A new link moves a record's clock forward from the stored value. It never
// folds the record's history, which made a bulk import for a busy contact cost
// the square of that history. Removing a link still folds, because it can move
// the clock back.
//
// The stored clocks are set past every activity first. A fold on insert would
// pull them back to the newest activity, and an advance leaves them be.
func TestALinkMovesTheClockForwardWithoutFoldingTheHistory(t *testing.T) {
	e := Setup(t)
	ctx := context.Background()
	owner := OwnerConn(t)
	company := e.SeedCompany(t, "Busy Account", nil)
	contact := e.SeedContact(t, "Busy Contact", nil)
	project := seedProjectInPhase(t, e, company, "Busy Project", projectPhaseInitiative)

	note := func(when time.Time) ids.UUID {
		t.Helper()
		subject := "touch"
		logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "note", Subject: &subject, OccurredAt: &when, Source: "manual",
			Links: []activities.ActivityLinkInput{
				{EntityType: "contact", EntityID: contact}, {EntityType: "project", EntityID: project},
			},
		})
		if err != nil {
			t.Fatalf("logging a note: %v", err)
		}
		return ids.UUID(logged.Id)
	}
	clocks := func() (time.Time, time.Time) {
		t.Helper()
		var onContact, onProject time.Time
		if err := owner.QueryRow(ctx, `SELECT (SELECT last_activity_at FROM contact WHERE id = $1),
			(SELECT last_activity_at FROM project WHERE id = $2)`, contact, project).Scan(&onContact, &onProject); err != nil {
			t.Fatalf("reading the clocks: %v", err)
		}
		return onContact.UTC(), onProject.UTC()
	}

	history := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	note(history)
	ahead := history.Add(48 * time.Hour)
	if _, err := owner.Exec(ctx, `UPDATE contact SET last_activity_at = $2 WHERE id = $1`, contact, ahead); err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `UPDATE project SET last_activity_at = $2 WHERE id = $1`, project, ahead); err != nil {
		t.Fatal(err)
	}

	note(history.Add(24 * time.Hour))
	if onContact, onProject := clocks(); !onContact.Equal(ahead) || !onProject.Equal(ahead) {
		t.Fatalf("an older note moved the clocks to %v and %v, want both left at %v: the insert folded the history",
			onContact, onProject, ahead)
	}

	newest := ahead.Add(24 * time.Hour)
	newestNote := note(newest)
	if onContact, onProject := clocks(); !onContact.Equal(newest) || !onProject.Equal(newest) {
		t.Fatalf("a newer note moved the clocks to %v and %v, want both at %v", onContact, onProject, newest)
	}

	if _, err := owner.Exec(ctx, `DELETE FROM activity_link WHERE activity_id = $1`, newestNote); err != nil {
		t.Fatal(err)
	}
	refolded := history.Add(24 * time.Hour)
	if onContact, onProject := clocks(); !onContact.Equal(refolded) || !onProject.Equal(refolded) {
		t.Errorf("after unlinking the newest note the clocks read %v and %v, want both refolded to %v",
			onContact, onProject, refolded)
	}
}
