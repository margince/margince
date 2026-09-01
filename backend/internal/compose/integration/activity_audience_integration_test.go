// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/person360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// An activity's audience is a per-message, human-set limit on who reads its
// CONTENT. Customer identity is workspace-readable, so a colleague in another
// team discovers the row through the contact — and sees it withheld: date,
// direction and kind, not subject or body. The author keeps reading it, a
// named member is let in, and nobody but a writer of the row may set it.
func TestLimitingAnActivityWithholdsItsContentFromEveryoneButItsAudience(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Dana Buyer", &e.Rep1)

	subject, body := "Q3 renewal terms", "confidential pricing"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))

	// Before: the colleague reads it whole.
	before, err := e.Activities.GetActivity(colleague, id, storekit.LiveOnly)
	if err != nil || before.Subject == nil || *before.ContentState != crmcontracts.ActivityContentStateAvailable {
		t.Fatalf("before limiting: %+v %v, want the subject and content_state=available", before, err)
	}

	// Nobody but a writer of the row may limit it.
	if _, err := e.Activities.SetAudience(colleague, id, activities.SetAudienceInput{Audience: "participants"}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a colleague limiting another team's mail → %v, want ErrPermissionDenied", err)
	}
	if _, err := e.Activities.SetAudience(e.AgentCtx(), id, activities.SetAudienceInput{Audience: "participants"}); err == nil {
		t.Error("an agent limited an activity; the audience is a human's call")
	}

	limited, err := e.Activities.SetAudience(author, id, activities.SetAudienceInput{Audience: "participants"})
	if err != nil {
		t.Fatalf("author limiting: %v", err)
	}
	if limited.Audience == nil || *limited.Audience != crmcontracts.ActivityAudienceParticipants || limited.Subject == nil {
		t.Errorf("the author's own read after limiting = %+v, want audience=participants with the content", limited)
	}

	// After: discoverable, content withheld — on the single read and on the list.
	after, err := e.Activities.GetActivity(colleague, id, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("colleague reading a limited mail: %v, want the withheld row", err)
	}
	if *after.ContentState != crmcontracts.ActivityContentStateWithheld || after.Subject != nil || after.Body != nil {
		t.Errorf("colleague's read = %+v, want content_state=withheld with no subject/body", after)
	}
	// The REASON travels with the content. Why a message is held describes what
	// it is about — "personnel", "legal", "security_incident" — so a colleague
	// who may not read it must not learn why it is held either. The field is
	// optional on the wire, so nothing downstream fails when it leaks.
	if after.AudienceReason != nil {
		t.Errorf("the withheld row told the colleague why it is held: %q", *after.AudienceReason)
	}
	if after.Direction == nil || after.OccurredAt.IsZero() {
		t.Errorf("the withheld row lost its safe markers: %+v", after)
	}
	page, _, err := e.Activities.ListActivities(colleague, activities.ListActivitiesInput{EntityType: strPtr("person"), EntityID: &contact})
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 1 || *page[0].ContentState != crmcontracts.ActivityContentStateWithheld || page[0].Subject != nil {
		t.Errorf("colleague's list = %+v, want the one row withheld", page)
	}
	if len(page) == 1 && page[0].AudienceReason != nil {
		t.Errorf("the withheld LIST row told the colleague why it is held: %q", *page[0].AudienceReason)
	}
	// The audit row carries no content either.
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE action = 'update' AND entity_id = $1 AND after::text LIKE '%'||$2||'%'`, logged.Id, body); n != 0 {
		t.Error("the audience audit row carries the body")
	}

	// Naming the colleague lets them in; re-opening lets everyone in.
	if _, err := e.Activities.SetAudience(author, id, activities.SetAudienceInput{Audience: "selected",
		Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep3}}}); err != nil {
		t.Fatalf("selecting the colleague: %v", err)
	}
	if got, err := e.Activities.GetActivity(colleague, id, storekit.LiveOnly); err != nil || got.Subject == nil {
		t.Errorf("a selected member's read = %+v %v, want the content", got, err)
	}
	if _, err := e.Activities.SetAudience(author, id, activities.SetAudienceInput{Audience: "workspace"}); err != nil {
		t.Fatal(err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM activity_audience_member WHERE activity_id = $1`, logged.Id); n != 0 {
		t.Errorf("%d member rows after re-opening, want 0", n)
	}
	if got, err := e.Activities.GetActivity(colleague, id, storekit.LiveOnly); err != nil || got.Subject == nil {
		t.Errorf("after re-opening, colleague's read = %+v %v, want the content", got, err)
	}
}

// The record page reads its timeline through the person 360, which assembles
// its OWN statement over `activity` rather than going through the shared
// projection. So it is a second writer of the same promise, and the promise has
// to be proved on it separately: a colleague reading a contact's page learns
// that a limited message exists, and nothing about what it is or why it is held.
//
// This is the read that shipped without the reason at all, which is the other
// half of the same bug — the field is optional on the wire, so both a dropped
// reason and a leaked one pass unnoticed by everything downstream.
func TestThePersonPageWithholdsALimitedMessagesReasonFromAColleague(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, activityLifecyclePerms)
	contact := e.SeedPerson(t, "Nadia Renewal", &e.Rep1)

	subject, body := "Aufhebungsvertrag draft", "terms nobody else is owed"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("outbound"),
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: contact}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(logged.Id))
	if _, err := e.Activities.SetAudience(author, id,
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting: %v", err)
	}

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()),
		func() time.Time { return roomFixedNow })

	// The AUTHOR's page carries the reason: it is their message, and the reason
	// is what the share decision is made against. Asserted first, so a version
	// that withholds it from everybody cannot pass the colleague's arm below.
	mine, err := svc.Assemble(author, ids.From[ids.PersonKind](contact))
	if err != nil {
		t.Fatalf("assembling the author's page: %v", err)
	}
	ownRow := onlyTimelineRow(t, mine)
	if ownRow.AudienceReason == nil {
		t.Error("the author's own page did not say why the message is held — the record page " +
			"is where that decision is made, so a reason it never receives is a reason nobody acts on")
	}

	theirs, err := svc.Assemble(colleague, ids.From[ids.PersonKind](contact))
	if err != nil {
		t.Fatalf("assembling the colleague's page: %v", err)
	}
	row := onlyTimelineRow(t, theirs)
	if row.ContentState == nil || *row.ContentState != crmcontracts.ActivityContentStateWithheld {
		t.Fatalf("the colleague's timeline row = %+v, want content_state=withheld", row)
	}
	if row.AudienceReason != nil {
		t.Errorf("the person page told a colleague why the message is held: %q — the reason "+
			"describes what the message is about", *row.AudienceReason)
	}
	if row.Subject != nil || row.Body != nil {
		t.Errorf("the person page carried a limited message's content to a colleague: %+v", row)
	}
}

// onlyTimelineRow takes the single activity a fixture put on the page. It
// fails rather than returning a zero row when the section is missing or holds
// a different number: an assertion against a row that is not there passes for
// the wrong reason, which is the failure this whole file exists to catch.
func onlyTimelineRow(t *testing.T, page crmcontracts.Person360) crmcontracts.Activity {
	t.Helper()
	if page.Activities == nil {
		t.Fatal("the page carried no activities section")
	}
	if len(page.Activities.Data) != 1 {
		t.Fatalf("the page carried %d activities, want the one the fixture logged",
			len(page.Activities.Data))
	}
	return page.Activities.Data[0]
}
