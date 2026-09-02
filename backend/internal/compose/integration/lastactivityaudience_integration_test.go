// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// last_activity_at stops counting mail nobody can read.
//
// Its sibling next door (lastactivity_integration_test.go) proves the clock
// moves correctly — the newest wins, a back-dated capture never moves it
// backwards, archiving moves it back. This proves the other rule on the same
// column: a message limited to its participants is not "contact", so it must
// not move a date every colleague sees.
//
// Four owner tables read that column, all from their own SQL helper, so all
// four are exercised. The project arm is the one an earlier draft of this
// change missed entirely.
//
// The narrowing goes through activities.SetAudience, the real writer, rather
// than an UPDATE: the triggers fire on the write, and a test that set the
// column by hand would be asserting its own seed.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestLastActivityAudience_NarrowingTheNewestMessageMovesTheClockBack(t *testing.T) {
	e := Setup(t)
	org := e.SeedOrg(t, "Held Clock Org", nil)
	person := e.SeedPerson(t, "Held Clock Contact", nil)
	personID := ids.From[ids.PersonKind](person)
	orgID := ids.From[ids.OrganizationKind](org)
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &orgID,
		IsCurrentPrimary: boolPtr(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Held Clock Deal", AmountMinor: int64Ptr(100), Currency: strPtr("EUR"),
		PipelineID: pipeline, StageID: open, OrganizationID: &orgID, Source: "manual",
	})
	if err != nil {
		t.Fatal(err)
	}
	project := e.seedProjectFor(t, orgID)

	older := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	links := []activities.ActivityLinkInput{
		{EntityType: "person", EntityID: person},
		{EntityType: "organization", EntityID: org},
		{EntityType: "deal", EntityID: ids.UUID(deal.Id)},
		{EntityType: "project", EntityID: project},
	}
	e.logAt(t, older, links...)
	newestMessage := e.logAt(t, newest, links...)

	// Every clock sits on the newest message while it is open. Asserted before
	// narrowing, so a later "the clock is at `older`" cannot pass because the
	// newest message never registered at all.
	for name, got := range e.everyClock(t, personID, orgID, ids.From[ids.DealKind](ids.UUID(deal.Id)), project) {
		if got == nil || !got.Equal(newest) {
			t.Fatalf("%s clock = %v before narrowing, want %v — the fixture never took effect",
				name, got, newest)
		}
	}

	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](newestMessage),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("narrowing the newest message: %v", err)
	}

	for name, got := range e.everyClock(t, personID, orgID, ids.From[ids.DealKind](ids.UUID(deal.Id)), project) {
		if got == nil || !got.Equal(older) {
			t.Errorf("%s clock = %v after narrowing the newest message, want %v: a colleague "+
				"who cannot read that message is still told it happened", name, got, older)
		}
	}
}

func TestLastActivityAudience_WideningTheMessageMovesTheClockForward(t *testing.T) {
	// The admit case, and the round trip. Without it a helper that counted
	// nothing at all would pass the test above.
	e := Setup(t)
	org := e.SeedOrg(t, "Widen Clock Org", nil)
	person := e.SeedPerson(t, "Widen Clock Contact", nil)
	personID := ids.From[ids.PersonKind](person)
	orgID := ids.From[ids.OrganizationKind](org)
	if _, err := e.People.CreateRelationship(e.Admin(), people.CreateRelationshipInput{
		Kind: "employment", PersonID: &personID, OrganizationID: &orgID,
		IsCurrentPrimary: boolPtr(true), Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}

	older := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	e.logAt(t, older, activities.ActivityLinkInput{EntityType: "person", EntityID: person})
	held := e.logAt(t, newest, activities.ActivityLinkInput{EntityType: "person", EntityID: person})

	heldID := ids.From[ids.ActivityKind](held)
	if _, err := e.Activities.SetAudience(e.Admin(), heldID,
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("narrowing: %v", err)
	}
	if got := e.personClock(t, personID); got == nil || !got.Equal(older) {
		t.Fatalf("clock = %v while the newest message is held, want %v", got, older)
	}

	if _, err := e.Activities.SetAudience(e.Admin(), heldID,
		activities.SetAudienceInput{Audience: "workspace"}); err != nil {
		t.Fatalf("widening: %v", err)
	}
	if got := e.personClock(t, personID); got == nil || !got.Equal(newest) {
		t.Errorf("clock = %v after re-opening the message, want %v: widening must move the "+
			"clock forward again, or a mistaken hold is permanent", got, newest)
	}
}

func TestLastActivityAudience_AMessageBornHeldNeverMovesTheClock(t *testing.T) {
	// The trigger arm the narrow-then-check case cannot reach. Captured mail is
	// born with its audience already set (capture/birthdecision.go), so nothing
	// ever UPDATEs the audience column for it — the value has to be right at
	// INSERT, through the link trigger rather than the activity one.
	e := Setup(t)
	person := e.SeedPerson(t, "Born Held Contact", nil)
	personID := ids.From[ids.PersonKind](person)

	older := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	e.logAt(t, older, activities.ActivityLinkInput{EntityType: "person", EntityID: person})
	born := e.logAt(t, newest, activities.ActivityLinkInput{EntityType: "person", EntityID: person})
	// Narrowed at the row before its link exists is not reproducible through the
	// public writers, so the audience is set directly and the LINK is then
	// re-driven — which is the order capture writes in, and the arm that proves
	// the helper filters rather than the activity trigger happening to fire.
	e.WsExec(t, `UPDATE activity SET audience = 'participants' WHERE id = $1`, born)
	e.WsExec(t, `UPDATE activity_link SET person_id = person_id WHERE activity_id = $1`, born)

	if got := e.personClock(t, personID); got == nil || !got.Equal(older) {
		t.Errorf("clock = %v for a message born held, want %v", got, older)
	}
}

// logAt writes one note at a moment, through the real writer.
func (e *Env) logAt(t *testing.T, when time.Time, links ...activities.ActivityLinkInput) ids.UUID {
	t.Helper()
	subject := "touch"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "note", Subject: &subject, OccurredAt: &when, Source: "manual", Links: links,
	})
	if err != nil {
		t.Fatalf("logging: %v", err)
	}
	return ids.UUID(logged.Id)
}

// everyClock reads all four stored last_activity_at values, keyed by the table
// they came from so a failure names which helper is wrong.
func (e *Env) everyClock(
	t *testing.T, person ids.PersonID, org ids.OrganizationID, deal ids.DealID, project ids.UUID,
) map[string]*time.Time {
	t.Helper()
	organization, err := e.People.GetOrganization(e.Admin(), org, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	got, err := e.Deals.GetDeal(e.Admin(), deal, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return map[string]*time.Time{
		"person":       e.personClock(t, person),
		"organization": organization.LastActivityAt,
		"deal":         got.LastActivityAt,
		"project":      e.projectClock(t, project),
	}
}

func (e *Env) personClock(t *testing.T, id ids.PersonID) *time.Time {
	t.Helper()
	person, err := e.People.GetPerson(e.Admin(), id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	return person.LastActivityAt
}

// projectClock reads the column directly: project has no read surface in this
// harness, and the column is what all four helpers write and every reader reads.
func (e *Env) projectClock(t *testing.T, id ids.UUID) *time.Time {
	t.Helper()
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	var at *time.Time
	if err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT last_activity_at FROM project WHERE id = $1`, id).Scan(&at)
	}); err != nil {
		t.Fatalf("reading the project clock: %v", err)
	}
	return at
}

func (e *Env) seedProjectFor(t *testing.T, org ids.OrganizationID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO project (id, name, organization_id, source, captured_by)
		VALUES ($1, 'Held Clock Project', $2, 'manual', 'human:test')`, id, org)
	return id
}
