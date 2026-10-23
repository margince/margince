// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestMeetingDetailNamesReadableAttendeesNotFiledContacts(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	id := e.logMeeting(t, "detail-invite", nil, storekit.SourceAuthorInput{AuthorID: &e.other})
	activityID := ids.From[ids.ActivityKind](id)
	attendee := e.linkContact(t, activityID, "Dana Buyer")
	filed := e.linkContact(t, activityID, "Filed Contact")
	for contact, role := range map[ids.UUID]string{attendee: "attendee", filed: "from"} {
		if _, err := e.owner.Exec(ctx, `INSERT INTO activity_participant(activity_id,contact_id,role) VALUES($1,$2,$3)`, id, contact, role); err != nil {
			t.Fatal(err)
		}
	}
	row, err := e.store(nil).GetActivity(ctx, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if row.MeetingParticipants == nil || len(*row.MeetingParticipants) != 1 || ids.UUID((*row.MeetingParticipants)[0].ContactId) != attendee || (*row.MeetingParticipants)[0].FullName != "Dana Buyer" {
		t.Fatalf("participants = %+v", row.MeetingParticipants)
	}
	if row.Links == nil || len(*row.Links) != 2 {
		t.Fatalf("filing links lost: %+v", row.Links)
	}
	if _, err := e.owner.Exec(ctx, `UPDATE contact SET owner_id=$2, visibility='owner', captured_by=$3 WHERE id=$1`, attendee, e.other, "human:"+e.other.String()); err != nil {
		t.Fatal(err)
	}
	scoped, err := e.store(nil).GetActivity(e.as(principal.RowScopeOwn), activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if scoped.MeetingParticipants != nil && len(*scoped.MeetingParticipants) != 0 {
		t.Fatal("participants escaped contact row scope")
	}
	noContacts := principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Read: true}}, RowScope: principal.RowScopeAll},
	})
	hidden, err := e.store(nil).GetActivity(noContacts, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.MeetingParticipants != nil && len(*hidden.MeetingParticipants) != 0 {
		t.Fatal("participants escaped contact read permission")
	}
	if _, err := e.store(nil).SetAudience(ctx, activityID, SetAudienceInput{Audience: "selected", Members: []AudienceMember{{SubjectType: "user", SubjectID: e.rep}}}); err != nil {
		t.Fatal(err)
	}
	other := principal.WithActor(ctx, principal.Principal{Type: principal.PrincipalHuman, ID: "human:" + e.other.String(), UserID: e.other, Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}}, RowScope: principal.RowScopeAll}})
	withheld, err := e.store(nil).GetActivity(other, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if withheld.ContentState == nil || *withheld.ContentState != "withheld" || withheld.MeetingParticipants != nil {
		t.Fatal("withheld meeting disclosed participants")
	}
}

func TestMeetingDetailReadPermissionDoesNotPermitAnUpdate(t *testing.T) {
	e := setupSend(t)
	id := e.logMeeting(t, "", nil, storekit.SourceAuthorInput{})
	activityID := ids.From[ids.ActivityKind](id)
	ctx := principal.WithActor(e.as(principal.RowScopeAll), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{Objects: map[string]principal.ObjectGrant{"activity": {Read: true}, "contact": {Read: true}}, RowScope: principal.RowScopeAll},
	})
	before, err := e.store(nil).GetActivity(ctx, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	_, err = e.store(nil).UpdateActivity(ctx, activityID, UpdateActivityInput{Subject: new("Changed"), IfVersion: before.Version})
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read-only update = %v", err)
	}
	after, err := e.store(nil).GetActivity(ctx, activityID, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if after.Version == nil || before.Version == nil || *after.Version != *before.Version || after.Subject == nil || before.Subject == nil || *after.Subject != *before.Subject {
		t.Fatal("read-only update changed the meeting")
	}
}
