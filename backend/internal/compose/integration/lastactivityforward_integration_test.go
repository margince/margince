// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
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

	agentAt := newest.Add(time.Hour)
	agentSubject, agentBody := "Scheduled agent note", "The deal has gone quiet for three weeks."
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &agentSubject, Body: &agentBody, OccurredAt: &agentAt, Source: "manual", Origin: activities.OriginAgent,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}, {EntityType: "project", EntityID: project}},
	}); err != nil {
		t.Fatal(err)
	}
	if onContact, onProject := clocks(); !onContact.Equal(newest) || !onProject.Equal(newest) {
		t.Fatalf("agent note moved the clocks to %v and %v, want both left at %v", onContact, onProject, newest)
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

// A contact's current employer counts only what was sent while they worked
// there. A new link advances that clock by the rule a refold reads. A note filed
// on the account itself carries no such bound.
func TestALinkAdvancesAnEmployerOnlyInsideTheEmployment(t *testing.T) {
	e := Setup(t)
	company := e.SeedCompany(t, "Newco", nil)
	contact := e.SeedContact(t, "Ann", nil)
	contactID, companyID := ids.From[ids.ContactKind](contact), ids.From[ids.CompanyKind](company)
	started := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "employment", ContactID: &contactID, CompanyID: &companyID, StartedAt: &started, Source: "manual",
	}); err != nil {
		t.Fatalf("recording the employment: %v", err)
	}
	note := func(when time.Time, links ...activities.ActivityLinkInput) {
		t.Helper()
		subject := "touch"
		if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
			Kind: "note", Subject: &subject, OccurredAt: &when, Source: "manual", Links: links,
		}); err != nil {
			t.Fatalf("logging a note: %v", err)
		}
	}
	onContact := activities.ActivityLinkInput{EntityType: "contact", EntityID: contact}
	onCompany := activities.ActivityLinkInput{EntityType: "company", EntityID: company}

	note(started.Add(-48*time.Hour), onContact)
	if got := companyClock(t, e, company); got != nil {
		t.Fatalf("company clock = %v from a note before the employment began, want none", got)
	}

	filed := started.Add(-24 * time.Hour)
	note(filed, onContact, onCompany)
	if got := companyClock(t, e, company); got == nil || !got.Equal(filed) {
		t.Fatalf("company clock = %v, want %v from a note filed on the account", got, filed)
	}

	inside := started.Add(24 * time.Hour)
	note(inside, onContact)
	if got := companyClock(t, e, company); got == nil || !got.Equal(inside) {
		t.Fatalf("company clock = %v, want %v from a note inside the employment", got, inside)
	}
}
