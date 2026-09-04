// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// What a PATCH on an activity writes into audit_log.before.
//
// Read against the real column rather than the Go value the store returned: the
// image is a jsonb document, and the question these tests ask is what a later
// reader — field history, and anything that restores a value — will find there.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// auditImagesFor reads the images of the newest update row for one activity.
func auditImagesFor(t *testing.T, e *sendEnv, activityID ids.UUID) (before, after map[string]any) {
	t.Helper()
	var beforeJSON, afterJSON []byte
	if err := e.owner.QueryRow(context.Background(),
		`SELECT before, after FROM audit_log
		  WHERE entity_type = 'activity' AND entity_id = $1 AND action = 'update'
		  ORDER BY occurred_at DESC, id DESC LIMIT 1`, activityID,
	).Scan(&beforeJSON, &afterJSON); err != nil {
		t.Fatalf("reading the audit row: %v", err)
	}
	if beforeJSON == nil {
		t.Fatal("before is SQL NULL — the row cannot say what the edit changed from")
	}
	if err := json.Unmarshal(beforeJSON, &before); err != nil {
		t.Fatalf("before is not an object: %v", err)
	}
	if err := json.Unmarshal(afterJSON, &after); err != nil {
		t.Fatalf("after is not an object: %v", err)
	}
	return before, after
}

// The note these tests edit. Its subject and body are named here because every
// test below changes one of them and asserts on what it was.
const (
	noteSubject = "Kickoff"
	noteBody    = "the original text"
)

func loggedNote(ctx context.Context, t *testing.T, e *sendEnv) crmcontracts.Activity {
	t.Helper()
	subject, body := noteSubject, noteBody
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "note", Subject: &subject, Body: &body, Source: "ui",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return activity
}

// The body is the field a reader most often wants back, so the image carries the
// text the patch replaced rather than a flag that it changed. A flag says an
// edit happened and leaves nobody able to say what it undid.
func TestAPatchedBodyRecordsTheTextItReplaced(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	activity := loggedNote(ctx, t, e)

	revised := "the revised text"
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)),
		UpdateActivityInput{Body: &revised}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	before, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if before["body"] != noteBody {
		t.Errorf("before[body] = %v, want the text the edit replaced", before["body"])
	}
	if after["body"] != revised {
		t.Errorf("after[body] = %v, want %q", after["body"], revised)
	}
}

// A field the patch did not touch must not appear in either image. Field history
// reads every key in the pair, so an untouched column carried through publishes
// a change nobody made.
func TestAnUntouchedFieldStaysOutOfBothImages(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	activity := loggedNote(ctx, t, e)

	renamed := noteSubject + ", revised"
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)),
		UpdateActivityInput{Subject: &renamed}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	before, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if before["subject"] != noteSubject {
		t.Errorf("before[subject] = %v, want the replaced subject", before["subject"])
	}
	if _, carried := before["body"]; carried {
		t.Errorf("body reached the before image on a subject-only edit: %v", before)
	}
	if _, carried := after["body"]; carried {
		t.Errorf("body reached the after image on a subject-only edit: %v", after)
	}
}

// done_at is stamped by the UPDATE statement, from the database's clock. The
// image is read back from the row for exactly this: a completion records the
// moment the row actually holds, and reopening records its removal, neither of
// which the incoming patch could have supplied.
func TestCompletingATaskRecordsTheStampTheRowReceived(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	subject := "Send the deck"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "task", Subject: &subject, Source: "ui",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	done := true
	updated, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)),
		UpdateActivityInput{IsDone: &done})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}
	if updated.DoneAt == nil {
		t.Fatal("completion did not stamp done_at; the rest of this test proves nothing")
	}

	before, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if before["is_done"] != false || after["is_done"] != true {
		t.Errorf("is_done image = %v -> %v, want false -> true", before["is_done"], after["is_done"])
	}
	if before["done_at"] != nil {
		t.Errorf("before[done_at] = %v, want nil — the task was not done", before["done_at"])
	}
	if after["done_at"] == nil {
		t.Error("after[done_at] is absent, so the image did not record the stamp the row received")
	}
}

// A transcript body is renormalized on the way in. The after image is read back
// from the row so it records the canonical form actually stored, not the raw
// text the caller sent.
func TestATranscriptPatchRecordsTheNormalizedBodyTheRowHolds(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	raw, sourceSystem := "Anna: hello", "transcript"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "call", Body: &raw, SourceSystem: &sourceSystem, Source: "ui",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}

	unnormalized := "Anna: hello   \r\nBen: hi\r\n"
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(activity.Id)),
		UpdateActivityInput{Body: &unnormalized}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	_, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if after["body"] != "Anna: hello\nBen: hi" {
		t.Errorf("after[body] = %v, want the normalized form the row stores", after["body"])
	}
}

// An audience write moves two things — the column on the activity row and the
// member rows that qualify it — so the images say what both held. Recording only
// what the audience became would leave the row unable to say who lost sight of
// the conversation.
func TestAnAudienceChangeRecordsWhatItNarrowedFrom(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	activity := loggedNote(ctx, t, e)
	activityID := ids.From[ids.ActivityKind](ids.UUID(activity.Id))

	if _, err := e.store(nil).SetAudience(ctx, activityID, SetAudienceInput{
		Audience: "selected",
		Members:  []AudienceMember{{SubjectType: "user", SubjectID: e.rep}},
	}); err != nil {
		t.Fatalf("SetAudience: %v", err)
	}

	before, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if before["audience"] != "workspace" {
		t.Errorf("before[audience] = %v, want the audience the write replaced", before["audience"])
	}
	if after["audience"] != "selected" {
		t.Errorf("after[audience] = %v, want selected", after["audience"])
	}
	if members := imageMembers(t, before); len(members) != 0 {
		t.Errorf("before[members] = %v, want the empty set a workspace audience holds", members)
	}
	if members := imageMembers(t, after); len(members) != 1 || members[0] != "user:"+e.rep.String() {
		t.Errorf("after[members] = %v, want the one member admitted", members)
	}
}

// Widening the member set leaves the audience column alone, and a column that
// did not move belongs in neither image: field history reads every key in the
// pair, so carrying it through would publish a narrowing nobody performed.
func TestAnUnchangedAudienceStaysOutOfBothImages(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	activity := loggedNote(ctx, t, e)
	activityID := ids.From[ids.ActivityKind](ids.UUID(activity.Id))

	for _, members := range [][]AudienceMember{
		{{SubjectType: "user", SubjectID: e.rep}},
		{{SubjectType: "user", SubjectID: e.rep}, {SubjectType: "user", SubjectID: e.other}},
	} {
		if _, err := e.store(nil).SetAudience(ctx, activityID, SetAudienceInput{
			Audience: "selected", Members: members,
		}); err != nil {
			t.Fatalf("SetAudience: %v", err)
		}
	}

	before, after := auditImagesFor(t, e, ids.UUID(activity.Id))
	if _, carried := before["audience"]; carried {
		t.Errorf("audience reached the before image on a members-only write: %v", before)
	}
	if _, carried := after["audience"]; carried {
		t.Errorf("audience reached the after image on a members-only write: %v", after)
	}
	if members := imageMembers(t, before); len(members) != 1 || members[0] != "user:"+e.rep.String() {
		t.Errorf("before[members] = %v, want the single-member set the write replaced", members)
	}
	if members := imageMembers(t, after); len(members) != 2 {
		t.Errorf("after[members] = %v, want both members", members)
	}
}

// imageMembers reads the member set out of one audit image, insisting it is a
// list of words rather than accepting whatever jsonb happened to hold.
func imageMembers(t *testing.T, image map[string]any) []string {
	t.Helper()
	raw, present := image["members"]
	if !present {
		t.Fatalf("the image carries no member set: %v", image)
	}
	list, isList := raw.([]any)
	if !isList {
		t.Fatalf("members = %v, want a list", raw)
	}
	members := make([]string, 0, len(list))
	for _, entry := range list {
		word, isWord := entry.(string)
		if !isWord {
			t.Fatalf("member %v is not a subject word", entry)
		}
		members = append(members, word)
	}
	return members
}

// loggedMeeting is a meeting with nothing recorded about how it went, which is
// every meeting until somebody says otherwise.
func loggedMeeting(ctx context.Context, t *testing.T, e *sendEnv) crmcontracts.Activity {
	t.Helper()
	subject := "Quarterly review"
	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "meeting", Subject: &subject, Source: "ui",
	})
	if err != nil {
		t.Fatalf("LogActivityInputFrom: %v", err)
	}
	activity, _, err := e.store(nil).LogActivity(ctx, in)
	if err != nil {
		t.Fatalf("LogActivity: %v", err)
	}
	return activity
}

// How a meeting went, recorded after it happened.
//
// The field was contracted on CREATE since it existed and absent from the
// patch, so the one moment a human actually knows the answer — the meeting is
// over — was the one moment the API could not be told. A rep whose meeting was
// a no-show had to log a second activity saying so in prose.
func TestHowAMeetingWentCanBeRecordedAfterIt(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	meeting := loggedMeeting(ctx, t, e)
	if meeting.MeetingStatus != nil {
		t.Fatalf("a freshly logged meeting already reports %v", *meeting.MeetingStatus)
	}

	held := string(crmcontracts.ActivityMeetingStatusHeld)
	out, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)),
		UpdateActivityInput{MeetingStatus: &held})
	if err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	if out.MeetingStatus == nil || string(*out.MeetingStatus) != held {
		t.Fatalf("the meeting reports %v, want %q", out.MeetingStatus, held)
	}
}

// And the trail says who recorded it, which is the question that trail is read
// for. A column the update can change and the diff cannot see leaves an audit
// row saying nothing happened.
func TestRecordingAMeetingsOutcomeReachesTheAuditImages(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	meeting := loggedMeeting(ctx, t, e)

	noShow := string(crmcontracts.ActivityMeetingStatusNoShow)
	if _, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)),
		UpdateActivityInput{MeetingStatus: &noShow}); err != nil {
		t.Fatalf("UpdateActivity: %v", err)
	}

	before, after := auditImagesFor(t, e, ids.UUID(meeting.Id))
	if before["meeting_status"] != nil {
		t.Errorf("before[meeting_status] = %v, want the nothing that was recorded",
			before["meeting_status"])
	}
	if after["meeting_status"] != noShow {
		t.Errorf("after[meeting_status] = %v, want %q — a write the trail cannot see "+
			"is a write nobody can attribute", after["meeting_status"], noShow)
	}
}

// Only a meeting has one, held against the kind the ROW carries.
//
// A patch cannot change a kind, so the stored one is the only honest thing to
// check. Without this a note given `held` would store silently and read back as
// a meeting-shaped fact about something that was not one — the pairing create
// already refuses, and which the database CHECK does not constrain.
func TestOnlyAMeetingMayBeToldHowItWent(t *testing.T) {
	e := setupSend(t)
	ctx := e.as(principal.RowScopeAll)
	note := loggedNote(ctx, t, e)

	held := string(crmcontracts.ActivityMeetingStatusHeld)
	_, err := e.store(nil).UpdateActivity(ctx, ids.From[ids.ActivityKind](ids.UUID(note.Id)),
		UpdateActivityInput{MeetingStatus: &held})

	var kindErr *MeetingStatusKindError
	if !errors.As(err, &kindErr) {
		t.Fatalf("a note was told how it went and answered %v", err)
	}
	field, code, _ := kindErr.FieldFault()
	if field != "meeting_status" || code != faultNotValidForKind {
		t.Errorf("the fault names %q/%q, want the field the caller has to drop",
			field, code)
	}
}
