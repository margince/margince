// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/dealstatus"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCurrentMovesStopNamingWorkAfterItsAudienceNarrows(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, &e.Rep1)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	subject := "Send the scope"
	task, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "task", Subject: &subject, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal}},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	moves, err := cards.CurrentMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	named, ok := dealstatus.NamedActivity(moves[deal])
	if !ok || named != ids.UUID(task.Id) {
		t.Fatalf("current task missing before audience change: %+v", moves)
	}
	if _, err := store.SetAudience(e.Admin(), ids.From[ids.ActivityKind](ids.UUID(task.Id)), activities.SetAudienceInput{
		Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.AdminUser}},
	}); err != nil {
		t.Fatal(err)
	}
	moves, err = cards.CurrentMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	if named, ok := dealstatus.NamedActivity(moves[deal]); ok && named == ids.UUID(task.Id) {
		t.Fatalf("restricted work still supplies an operand: %+v", moves)
	}
}

func TestCurrentMovesWithholdAdviceWhenTheActivityGrantIsLost(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, &e.Rep1)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	permissions := integration.AdminPerms
	permissions.Objects = map[string]principal.ObjectGrant{"deal": {Read: true}, "installation_settings": {Read: true}}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, permissions)
	if _, err := cards.CurrentMoves(reader, []ids.UUID{deal}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("absence-based advice without access to its facts: %v", err)
	}
	permissions.Objects = map[string]principal.ObjectGrant{"activity": {Read: true}}
	reader = e.As(e.Rep1, []ids.UUID{e.Team1}, permissions)
	if _, err := cards.CurrentMoves(reader, []ids.UUID{deal}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("advice without the deal grant: %v", err)
	}
	if _, err := cards.CachedCards(reader, []ids.UUID{deal}); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("historical assessment without the deal grant: %v", err)
	}
}

func TestCurrentMovesStopNamingAContactAfterCapturePrivacyChanges(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Renewal", pipeline, stage, &e.Rep1))
	contact := ids.From[ids.ContactKind](e.SeedContact(t, "Demo champion", &e.Rep1))
	role := "champion"
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contact, DealID: &deal, Role: &role, Source: "manual",
	}); err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	calledAt, subject := at.Add(-48*time.Hour), "Scope discussion"
	if _, _, err := e.Activities.LogActivity(reader, activities.LogActivityInput{
		Kind: "call", Subject: &subject, OccurredAt: &calledAt, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "deal", EntityID: deal.UUID}, {EntityType: "contact", EntityID: contact.UUID}},
	}); err != nil {
		t.Fatal(err)
	}
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	moves, err := cards.CurrentMoves(reader, []ids.UUID{deal.UUID})
	if err != nil {
		t.Fatal(err)
	}
	named := dealstatus.NamedContacts(moves[deal.UUID])
	if len(named) != 1 || named[0] != contact.UUID {
		t.Fatalf("the current champion was never offered: %+v", moves)
	}
	e.MakeCapturePrivate(t, "contact", contact.UUID, e.Rep3)
	moves, err = cards.CurrentMoves(reader, []ids.UUID{deal.UUID})
	if err != nil {
		t.Fatal(err)
	}
	if named := dealstatus.NamedContacts(moves[deal.UUID]); len(named) != 0 {
		t.Fatalf("private contact still supplies a task target: %+v", moves)
	}
	if strings.Contains(moves[deal.UUID].Reason, "Demo champion") {
		t.Fatal("private contact's name survived in the recommendation")
	}
}

func TestCurrentMeetingFollowUpChangesAfterTheActualMailWriterRuns(t *testing.T) {
	e := integration.Setup(t)
	pipeline, stage, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "Renewal", pipeline, stage, &e.Rep1)
	contact := e.SeedContact(t, "Demo attendee", &e.Rep1)
	at := time.Date(2026, 10, 12, 9, 0, 0, 0, time.UTC)
	store := activities.NewStore(e.DB()).WithClock(func() time.Time { return at })
	meetingAt, status, duration, subject := at.Add(-72*time.Hour), "held", 3600, "Scope review"
	host := ids.From[ids.UserKind](e.Rep1)
	meeting, _, err := store.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &meetingAt, MeetingStatus: &status, DurationSeconds: &duration,
		HostUserID: &host, Source: "manual", Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: deal}, {EntityType: "contact", EntityID: contact},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	reader := e.As(e.Rep1, []ids.UUID{e.Team1}, integration.AdminPerms)
	cards := newDealStatusService(e.Pool, func() time.Time { return at })
	moves, err := cards.CurrentMoves(reader, []ids.UUID{deal})
	if err != nil || moves[deal].Action != dealstatus.ActionDraftEmail {
		t.Fatalf("the follow-up was never current: %+v, %v", moves, err)
	}
	direction, sentAt := "outbound", at.Add(-time.Hour)
	if _, _, err := store.LogActivity(reader, activities.LogActivityInput{
		Kind: "email", Direction: &direction, Subject: &subject, OccurredAt: &sentAt, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact}},
	}); err != nil {
		t.Fatal(err)
	}
	moves, err = cards.CurrentMoves(reader, []ids.UUID{deal})
	if err != nil {
		t.Fatal(err)
	}
	if named, ok := dealstatus.NamedActivity(moves[deal]); ok && named == ids.UUID(meeting.Id) {
		t.Fatalf("answered meeting still supplies a follow-up operand: %+v", moves)
	}
	if moves[deal].Action == dealstatus.ActionDraftEmail {
		t.Fatalf("settled follow-up is still offered: %+v", moves)
	}
}
