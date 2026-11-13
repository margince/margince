// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// transcriptOf logs a transcript on the create wire, as the Log form sends it.
// It names the meeting it records and is filed on the customer and the deal.
func transcriptOf(t *testing.T, e *sendEnv, meeting ids.ActivityID, customer, deal ids.UUID) (crmcontracts.Activity, error) {
	t.Helper()
	links := fmt.Sprintf(`[{"entity_type": "contact", "entity_id": %q}, {"entity_type": "deal", "entity_id": %q}]`, customer, deal)
	return transcriptAs(meetingCtx(e), t, e, meeting, links, "")
}

// transcriptAs is transcriptOf as the caller ctx, with these links and any
// further create fields.
func transcriptAs(ctx context.Context, t *testing.T, e *sendEnv, meeting ids.ActivityID, links, more string) (crmcontracts.Activity, error) {
	t.Helper()
	wire := fmt.Sprintf(`{"kind": "meeting", "source": "manual", "source_system": "transcript",
		"subject": "Review call", "body": "Buyer: we renew.\nRep: great.", "meeting_status": "held",
		"occurred_at": %q, "occurred_precision": "instant", "records_meeting_id": %q, "links": %s %s}`,
		clocktest.Now(t).Format(time.RFC3339), meeting, links, more)
	var req crmcontracts.CreateActivityRequest
	if err := json.Unmarshal([]byte(wire), &req); err != nil {
		t.Fatalf("decoding the create body: %v", err)
	}
	in, err := LogActivityInputFrom(req)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	in.LinksFromRequest = true
	out, _, err := meetingStore(e).LogActivity(ctx, in)
	return out, err
}

// transcriptCtx is meetingCtx's rep reading leads too, within rows.
func transcriptCtx(e *sendEnv, rows principal.RowScope) context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), e.ws)
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RoleKeys: []string{"rep"},
			Objects: map[string]principal.ObjectGrant{
				"activity": {Read: true, Create: true, Update: true},
				"contact":  {Read: true}, "deal": {Read: true}, "lead": {Read: true},
				"company": {Read: true}, "project": {Read: true},
			},
			RowScope: rows,
		},
	})
}

// colleagueMeeting is a booked meeting a colleague hosts and logged, seen by
// whoever its audience lets in.
func colleagueMeeting(t *testing.T, e *sendEnv, customer ids.UUID, audience string) ids.ActivityID {
	t.Helper()
	id := ids.NewV7()
	ctx := context.Background()
	if _, err := e.owner.Exec(ctx, `INSERT INTO activity (id, kind, subject, occurred_at, meeting_status, host_user_id, audience, source, captured_by)
		VALUES ($1, 'meeting', 'Their review', now() - interval '2 hours', 'booked', $2, $3, 'manual', $4)`,
		id, e.other, audience, "human:"+e.other.String()); err != nil {
		t.Fatalf("seeding the colleague's meeting: %v", err)
	}
	if _, err := e.owner.Exec(ctx, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`,
		id, customer); err != nil {
		t.Fatalf("filing the colleague's meeting: %v", err)
	}
	return ids.From[ids.ActivityKind](id)
}

func heldMeetingsOnDeal(t *testing.T, e *sendEnv, deal ids.UUID) int {
	t.Helper()
	var held int
	if err := e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.deal_id = $1
		WHERE a.kind = 'meeting' AND `+meetingoutcome.HeldSQL("a"), deal).Scan(&held); err != nil {
		t.Fatalf("counting the held meetings: %v", err)
	}
	return held
}

func TestATranscriptOfABookedMeetingMarksThatMeetingHeldAndCountsOnce(t *testing.T) {
	e := setupSend(t)
	customer, deal := seedCustomer(t, e), e.seedDeal(t, "Renewal")
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	meeting := customerMeeting(t, e, customer, "Quarterly review", clocktest.Now(t).Add(-2*time.Hour), &booked)

	transcript, err := transcriptOf(t, e, meeting, customer, deal)
	if err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	if transcript.SourceActivityId == nil || ids.UUID(*transcript.SourceActivityId) != meeting.UUID {
		t.Errorf("the transcript names source %v, want the meeting it records", transcript.SourceActivityId)
	}
	if transcript.MeetingStatus != nil {
		t.Errorf("the transcript carries outcome %v, want none: the outcome is the meeting's", *transcript.MeetingStatus)
	}
	got, err := meetingStore(e).GetActivity(meetingCtx(e), meeting, storekit.LiveOnly)
	if err != nil {
		t.Fatalf("reading the meeting: %v", err)
	}
	if got.MeetingStatus == nil || *got.MeetingStatus != crmcontracts.ActivityMeetingStatusHeld {
		t.Errorf("the meeting reads %v, want held", got.MeetingStatus)
	}
	if len(linksOfKinds(got, []string{linkEntityDeal})) != 1 {
		t.Errorf("the meeting is filed under %v, want the transcript's deal", got.Links)
	}
	now := clocktest.Now(t)
	kind := string(crmcontracts.ActivityKindMeeting)
	lane, _, err := meetingStore(e).ListActivities(meetingCtx(e), ListActivitiesInput{Kind: &kind, AwaitingOutcomeAsOf: &now})
	if err != nil {
		t.Fatalf("reading the outcome lane: %v", err)
	}
	for _, row := range lane {
		t.Errorf("%q still waits for an outcome", *row.Subject)
	}
	if held := heldMeetingsOnDeal(t, e, deal); held != 1 {
		t.Errorf("the deal counts %d held meetings, want 1", held)
	}
}

func TestATranscriptCannotRecordAMeetingThatAlreadyHasItsOutcome(t *testing.T) {
	e := setupSend(t)
	customer, deal := seedCustomer(t, e), e.seedDeal(t, "Renewal")
	held := string(crmcontracts.ActivityMeetingStatusHeld)
	meeting := customerMeeting(t, e, customer, "Quarterly review", clocktest.Now(t).Add(-2*time.Hour), &held)

	_, err := transcriptOf(t, e, meeting, customer, deal)
	var refused *followUpFieldError
	if !errors.As(err, &refused) || refused.Field != fieldRecordsMeetingID {
		t.Fatalf("recording a held meeting answered %v, want a refusal on %s", err, fieldRecordsMeetingID)
	}
	if count := heldMeetingsOnDeal(t, e, deal); count != 0 {
		t.Errorf("the refused create left %d held meetings on the deal, want none", count)
	}
}

// A transcript's next step follows from the meeting it records, as the typed
// notes' does, so the week counts that meeting as having one.
func TestATranscriptsFollowUpCitesTheMeetingItRecords(t *testing.T) {
	e := setupSend(t)
	customer, deal := seedCustomer(t, e), e.seedDeal(t, "Renewal")
	booked := string(crmcontracts.ActivityMeetingStatusBooked)
	meeting := customerMeeting(t, e, customer, "Quarterly review", clocktest.Now(t).Add(-2*time.Hour), &booked)
	links := fmt.Sprintf(`[{"entity_type": "contact", "entity_id": %q}, {"entity_type": "deal", "entity_id": %q}]`, customer, deal)
	if _, err := transcriptAs(meetingCtx(e), t, e, meeting, links, `, "follow_up_task": {"subject": "Send the quote"}`); err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	var cited int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity WHERE kind = 'task' AND source_activity_id = $1`, meeting.UUID).Scan(&cited); err != nil {
		t.Fatal(err)
	}
	if cited != 1 {
		t.Errorf("%d tasks cite the meeting, want the transcript's one follow-up", cited)
	}
}

// The target meeting is admitted as any write to it is: unseen is not found,
// seen but not the caller's to change is refused.
func TestATranscriptRecordsOnlyAMeetingTheCallerMayChange(t *testing.T) {
	e := setupSend(t)
	customer := seedCustomer(t, e)
	links := fmt.Sprintf(`[{"entity_type": "contact", "entity_id": %q}]`, customer)
	hidden := colleagueMeeting(t, e, customer, "participants")
	if _, err := transcriptAs(meetingCtx(e), t, e, hidden, links, ""); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a meeting the caller cannot see answered %v, want not found", err)
	}
	theirs := colleagueMeeting(t, e, customer, "workspace")
	if _, err := transcriptAs(transcriptCtx(e, principal.RowScopeOwn), t, e, theirs, links, ""); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a colleague's meeting answered %v, want permission denied", err)
	}
	var open int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM activity WHERE id = ANY($1) AND meeting_status = 'booked'`,
		[]ids.UUID{hidden.UUID, theirs.UUID}).Scan(&open); err != nil {
		t.Fatal(err)
	}
	if open != 2 {
		t.Errorf("%d of the two refused meetings are still booked, want both", open)
	}
}

// A meeting filed on a lead files its transcript there too, so the reading's
// proposals land on the lead.
func TestATranscriptOfALeadsMeetingIsFiledOnTheLead(t *testing.T) {
	e := setupSend(t)
	lead := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		VALUES ($1, 'Lead buyer', 'new', 'manual', $2, $3)`, lead, "human:"+e.rep.String(), e.rep); err != nil {
		t.Fatalf("seeding the lead: %v", err)
	}
	booked, start := string(crmcontracts.ActivityMeetingStatusBooked), clocktest.Now(t).Add(-2*time.Hour)
	subject := "Discovery"
	ctx := transcriptCtx(e, principal.RowScopeAll)
	meeting, _, err := meetingStore(e).LogActivity(ctx, LogActivityInput{
		Kind: string(crmcontracts.ActivityKindMeeting), Subject: &subject, OccurredAt: &start,
		MeetingStatus: &booked, Source: "manual", Links: []ActivityLinkInput{{EntityType: linkEntityLead, EntityID: lead}},
	})
	if err != nil {
		t.Fatalf("booking the lead's meeting: %v", err)
	}
	id := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	transcript, err := transcriptAs(ctx, t, e, id, fmt.Sprintf(`[{"entity_type": "lead", "entity_id": %q}]`, lead), "")
	if err != nil {
		t.Fatalf("logging the transcript: %v", err)
	}
	if len(linksOfKinds(transcript, []string{linkEntityLead})) != 1 {
		t.Errorf("the transcript is filed under %v, want the lead", transcript.Links)
	}
	held, err := meetingStore(e).GetActivity(ctx, id, storekit.LiveOnly)
	if err != nil {
		t.Fatal(err)
	}
	if held.MeetingStatus == nil || *held.MeetingStatus != crmcontracts.ActivityMeetingStatusHeld {
		t.Errorf("the lead's meeting reads %v, want held", held.MeetingStatus)
	}
	if len(linksOfKinds(held, []string{linkEntityLead})) != 1 {
		t.Errorf("the lead's meeting is filed under %v, want the lead once", held.Links)
	}
}
