// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestInvitationConflictIsAnActionableHTTPResponse(t *testing.T) {
	f := newInvitationFixture(t)
	booked := f.book(t)
	handlers := Handlers{store: f.store}
	payload, err := json.Marshal(f.request)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/scheduling/invitations", bytes.NewReader(payload)).WithContext(f.ctx)
	reply := httptest.NewRecorder()
	handlers.CreateMeetingInvitation(reply, req, crmcontracts.CreateMeetingInvitationParams{})
	assertSlotConflict(t, reply)
	start, end := monday(13), monday(14)
	other := f.request
	other.Start, other.End = start, end
	if _, err := f.store.CreateInvitation(f.ctx, other); err != nil {
		t.Fatal(err)
	}
	change, err := json.Marshal(crmcontracts.MeetingInvitationChange{Action: "reschedule", Version: booked.Version, Start: &start, End: &end})
	if err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest(http.MethodPatch, "/v1/scheduling/invitations/meeting", bytes.NewReader(change)).WithContext(f.ctx)
	reply = httptest.NewRecorder()
	handlers.ChangeMeetingInvitation(reply, req, booked.Id)
	assertSlotConflict(t, reply)
}

func assertSlotConflict(t *testing.T, reply *httptest.ResponseRecorder) {
	t.Helper()
	var problem struct {
		Code string `json:"code"`
	}
	if err := json.Unmarshal(reply.Body.Bytes(), &problem); err != nil {
		t.Fatal(err)
	}
	if reply.Code != http.StatusConflict || problem.Code != "slot_taken" {
		t.Fatalf("conflict: %d %s", reply.Code, reply.Body.String())
	}
}

func TestAuthoritativeProviderAbsenceCompletesCancellation(t *testing.T) {
	f := newInvitationFixture(t)
	f.calendar.fail = true
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := f.store.DeliverInvitations(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(10 * time.Minute)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "cancel", Version: meeting.Version}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil || meeting.Status != invitationCanceled {
		t.Fatalf("cancellation: %+v %v", meeting, err)
	}
	if _, err := f.store.CreateInvitation(f.ctx, f.request); err != nil {
		t.Fatalf("cancellation kept reservation: %v", err)
	}
}

func TestUnchangedProviderReconciliationPreservesVersionAndHistory(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	var before int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM audit_log`).Scan(&before); err != nil {
		t.Fatal(err)
	}
	f.now = f.now.Add(20 * time.Minute)
	if err := f.store.ReconcileInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := f.env.owner.QueryRow(f.ctx, `SELECT count(*) FROM audit_log`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if after.Version != meeting.Version || count != before {
		t.Fatalf("unchanged reconciliation wrote domain state: version %d→%d, audit %d→%d", meeting.Version, after.Version, before, count)
	}
}

func TestPublicRequestRecoveryReturnsTheSameMeetingAndCapability(t *testing.T) {
	f := newInvitationFixture(t)
	body := crmcontracts.BookPublicMeetingJSONRequestBody{}
	body.Booker.Email = "guest@example.test"
	req := httptest.NewRequest(http.MethodPost, "/public/booking/host", nil)
	req.Header.Set("Idempotency-Key", "2943cb96-b3c5-4445-8685-20d6fe8bb026")
	intent, err := publicBookingRequestIntent(req, "host", body)
	if err != nil {
		t.Fatal(err)
	}
	host, err := schedulingHost(f.ctx)
	if err != nil {
		t.Fatal(err)
	}
	created, err := f.store.reserveAndQueueInvitation(f.ctx, host, f.request, invitationIntent{Public: intent})
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := f.store.recoverPublicBooking(f.ctx, host, intent)
	if err != nil || recovered == nil {
		t.Fatalf("lost response unrecoverable: %v", err)
	}
	if recovered.Id != created.Id || recovered.ManagementToken == nil || *recovered.ManagementToken != *created.ManagementToken {
		t.Fatal("recovery replaced the meeting or guest capability")
	}
	intent.Digest = "different request"
	if _, err := f.store.recoverPublicBooking(f.ctx, host, intent); err == nil {
		t.Fatal("request key replayed different intent")
	}
}

func TestCalendarReceiptDoesNotTakeAnAttendeeCopiesNaturalKey(t *testing.T) {
	f := newInvitationFixture(t)
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	provider, event := "gcal", ids.UUID(meeting.Id).String()
	subject := "Attendee calendar copy"
	if _, _, err := f.store.LogActivity(f.ctx, LogActivityInput{Kind: KindMeeting, Subject: &subject, SourceSystem: &provider, SourceID: &event, Source: "gcal"}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	after, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil || after.Status != invitationConfirmed {
		t.Fatalf("attendee copy stalled organizer delivery: %+v %v", after, err)
	}
}

func TestRetryStartsANewBoundedDeliveryAttempt(t *testing.T) {
	f := newInvitationFixture(t)
	f.calendar.lookupErr = connector.ErrUnreachable
	meeting, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	for range 5 {
		if err := f.store.DeliverInvitations(f.ctx); err != nil {
			t.Fatal(err)
		}
		f.now = f.now.Add(10 * time.Minute)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(meeting.Id), crmcontracts.MeetingInvitationChange{Action: "retry", Version: meeting.Version}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	meeting, err = f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil || meeting.Status != invitationPending {
		t.Fatalf("retry inherited exhausted attempts: %+v %v", meeting, err)
	}
}

func TestCancellationOvertakingProviderAcceptanceCancelsTheLateEvent(t *testing.T) {
	f := newInvitationFixture(t)
	created, err := f.store.CreateInvitation(f.ctx, f.request)
	if err != nil {
		t.Fatal(err)
	}
	claimed, found, err := f.store.claimInvitation(f.ctx)
	if err != nil || !found {
		t.Fatalf("delivery claim: %v %v", found, err)
	}
	current, err := f.store.Invitation(f.ctx, ids.UUID(created.Id))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.store.ChangeInvitation(f.ctx, ids.UUID(created.Id), crmcontracts.MeetingInvitationChange{Action: "cancel", Version: current.Version}); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	current, err = f.store.Invitation(f.ctx, ids.UUID(created.Id))
	if err != nil || current.Status != invitationCanceling {
		t.Fatalf("cancellation trusted absence while save was in flight: %+v %v", current, err)
	}
	receipt := &connector.CalendarReceipt{EventID: "late-event"}
	if err := f.store.finishInvitation(f.ctx, claimed, receipt, nil); err != nil {
		t.Fatal(err)
	}
	if err := f.store.DeliverInvitations(f.ctx); err != nil {
		t.Fatal(err)
	}
	current, err = f.store.Invitation(f.ctx, ids.UUID(created.Id))
	if err != nil || current.Status != invitationCanceled || f.calendar.canceled != 1 {
		t.Fatalf("late event survived cancellation: %+v %v", current, err)
	}
}

func TestCalendarAuthorityLossRequiresAttentionWithoutCancelingTheMeeting(t *testing.T) {
	f := newInvitationFixture(t)
	meeting := f.book(t)
	f.calendar.inspectErr = connector.ErrAuthRejected
	f.now = f.now.Add(20 * time.Minute)
	if err := f.store.ReconcileInvitations(f.ctx); err == nil {
		t.Fatal("lost calendar authority was hidden")
	}
	current, err := f.store.Invitation(f.ctx, ids.UUID(meeting.Id))
	if err != nil || current.Status != invitationNeedsAttention {
		t.Fatalf("calendar loss: %+v %v", current, err)
	}
	if _, err := f.store.CreateInvitation(f.ctx, f.request); err == nil {
		t.Fatal("lost authority released an existing reservation")
	}
}
