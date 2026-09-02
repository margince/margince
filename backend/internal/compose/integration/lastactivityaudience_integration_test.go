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
	// The LINK trigger's arm, which the narrow-then-check case cannot reach.
	// Captured mail is born with its audience already decided
	// (capture/birthdecision.go), so nothing ever UPDATEs the audience column
	// for it — the value has to be right when the LINK appears, through
	// trg_activity_link_last_activity rather than the activity trigger.
	//
	// So the audience is set while the activity has no links at all, and the
	// link is inserted afterwards. Setting it after a link existed would fire
	// the activity trigger and move the clock back for the wrong reason,
	// leaving this test green against a link trigger that did nothing.
	e := Setup(t)
	person := e.SeedPerson(t, "Born Held Contact", nil)
	personID := ids.From[ids.PersonKind](person)

	older := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	e.logAt(t, older, activities.ActivityLinkInput{EntityType: "person", EntityID: person})

	// Unlinked, so no clock has heard of it yet.
	born := ids.NewV7()
	e.WsExec(t, `
		INSERT INTO activity (id, kind, subject, audience, occurred_at, source, captured_by)
		VALUES ($1, 'note', 'born held', 'participants', $2, 'manual', 'human:test')`,
		born, newest)
	if got := e.personClock(t, personID); got == nil || !got.Equal(older) {
		t.Fatalf("clock = %v before the link exists, want %v — the fixture is not isolating "+
			"the link trigger", got, older)
	}

	e.WsExec(t, `
		INSERT INTO activity_link (activity_id, entity_type, person_id)
		VALUES ($1, 'person', $2)`, born, person)

	if got := e.personClock(t, personID); got == nil || !got.Equal(older) {
		t.Errorf("clock = %v after linking a message that was born held, want %v: the link "+
			"trigger counted an activity nobody but its participants may read", got, older)
	}
}

func TestLastActivityAudience_AClockMoveIsNotAnEdit(t *testing.T) {
	// A narrowing moves four stored dates, and moving one must not look like
	// somebody edited the record. set_updated_at_bump_version() fires on all
	// four tables and suppresses itself only while margince.last_activity_move
	// is on — which move_last_activity sets and a bare UPDATE does not. A
	// recompute written as a plain UPDATE would bump version on every deal,
	// organization, person and project in the installation, invalidating every
	// If-Match a client holds.
	//
	// Its sibling next door asserts the same property for an ordinary clock
	// move; this is the audience-driven one, which reaches the same movers by a
	// different route.
	e := Setup(t)
	person := e.SeedPerson(t, "Version Clock Contact", nil)
	personID := ids.From[ids.PersonKind](person)

	older := time.Date(2026, 8, 1, 9, 0, 0, 0, time.UTC)
	newest := time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)
	e.logAt(t, older, activities.ActivityLinkInput{EntityType: "person", EntityID: person})
	held := e.logAt(t, newest, activities.ActivityLinkInput{EntityType: "person", EntityID: person})

	before, err := e.People.GetPerson(e.Admin(), personID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if before.Version == nil {
		t.Fatal("a created person carries a version")
	}

	if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](held),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("narrowing: %v", err)
	}

	after, err := e.People.GetPerson(e.Admin(), personID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if after.LastActivityAt == nil || !after.LastActivityAt.Equal(older) {
		t.Fatalf("clock = %v, want %v — the narrowing did not move it, so this proved nothing",
			after.LastActivityAt, older)
	}
	if after.Version == nil || *after.Version != *before.Version {
		t.Errorf("person.version = %v after a clock move, want %v unchanged: a client holding "+
			"the old version would be refused an edit it is entitled to make",
			after.Version, *before.Version)
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
