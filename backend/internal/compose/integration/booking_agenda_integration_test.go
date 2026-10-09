// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/compose/integration/jobtest"
)

// bookedInvitation is the part of a public booking's answer these tests follow.
type bookedInvitation struct {
	Invitation struct {
		ID    string `json:"id"`
		Token string `json:"management_token"`
	} `json:"invitation"`
}

// The agenda a booker types is written for the host, and the meeting record keeps
// it. The invitation the host's calendar sends never carries it, on creation or on
// a reschedule: its recipient is only an address the booker typed.
func TestABookersAgendaStaysOnTheRecordAndOffTheInvitation(t *testing.T) {
	e, provider := setupBookingProvider(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	deliver := startMeetingDelivery(t, e)
	base := "/v1/public/booking/" + bookingSlug(t, e)
	monday := nextMonday()
	const agenda = "Bring the renewal figures"
	booking := func(start time.Time) AnyMap {
		return AnyMap{
			"start": start, "end": start.Add(30 * time.Minute), "subject": agenda,
			"booker":  AnyMap{"name": "Agenda Guest", "email": "agenda@visitor.example"},
			"consent": AnyMap{"policy_version": "2026-01", "wording": "Contact me about this meeting."},
		}
	}

	// The published page sends a recovery key and a hand-written request need not,
	// so both shapes are held to the rule.
	var keyless, keyed bookedInvitation
	if status := publicCall(t, e, "POST", base, booking(monday.Add(time.Hour)), nil, &keyless); status != http.StatusCreated {
		t.Fatalf("booking without a recovery key → %d", status)
	}
	recovery := map[string]string{"Idempotency-Key": "3b2f6c1e-8d4a-4f7b-9c2e-5a1d7e3f9b60"}
	if status := publicCall(t, e, "POST", base, booking(monday.Add(2*time.Hour)), recovery, &keyed); status != http.StatusCreated {
		t.Fatalf("booking with a recovery key → %d", status)
	}
	deliver()
	for _, booked := range []bookedInvitation{keyless, keyed} {
		assertInvitationOmitsAgenda(t, provider, booked, agenda)
		var body string
		if err := e.Owner.QueryRow(context.Background(), `SELECT body FROM activity WHERE id = $1`, booked.Invitation.ID).Scan(&body); err != nil {
			t.Fatal(err)
		}
		if body != agenda {
			t.Fatalf("the meeting record holds %q, want the booker's agenda", body)
		}
	}

	// A reschedule writes the event again, so it answers to the same rule.
	path := "/v1/public/meeting/" + keyless.Invitation.Token
	var state struct {
		Status  string    `json:"status"`
		Version int       `json:"version"`
		Start   time.Time `json:"start"`
	}
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != http.StatusOK || state.Status != "confirmed" {
		t.Fatalf("delivered booking: %d %+v", status, state)
	}
	moved := monday.Add(4 * time.Hour)
	reschedule := AnyMap{"action": "reschedule", "version": state.Version, "start": moved, "end": moved.Add(30 * time.Minute)}
	if status := publicCall(t, e, "PATCH", path, reschedule, nil, nil); status != http.StatusAccepted {
		t.Fatalf("guest reschedule → %d", status)
	}
	deliver()
	if status := publicCall(t, e, "GET", path, nil, nil, &state); status != http.StatusOK || state.Status != "confirmed" || !state.Start.Equal(moved) {
		t.Fatalf("rescheduled booking: %d %+v", status, state)
	}
	assertInvitationOmitsAgenda(t, provider, keyless, agenda)
}

// The rule follows who wrote the words, not which door accepted the meeting. A
// proposal's description is its host's, so accepting it anonymously still sends it.
func TestAnAcceptedProposalStillSendsTheDescriptionItsHostWrote(t *testing.T) {
	e, provider := setupBookingProvider(t)
	e.BootstrapWorkspace(t)
	enableBookingPage(t, e)
	deliver := startMeetingDelivery(t, e)
	var contact struct {
		ID string `json:"id"`
	}
	guest := AnyMap{"source": "manual", "full_name": "Proposal Guest", "emails": []AnyMap{{"email": "guest@visitor.example", "is_primary": true}}}
	if status := e.Call(t, "POST", "/v1/contacts", guest, nil, &contact); status != http.StatusCreated {
		t.Fatalf("contact: %d", status)
	}
	const hostAgenda = "Walk through the migration plan"
	request := AnyMap{"contact_id": contact.ID, "attendee_email": "guest@visitor.example", "subject": "Project review", "description": hostAgenda, "duration_minutes": 30, "options": []AnyMap{}}
	var proposal struct {
		URL string `json:"url"`
	}
	if status := e.Call(t, "POST", "/v1/scheduling/proposals", request, nil, &proposal); status != http.StatusCreated {
		t.Fatalf("proposal: %d", status)
	}
	_, token, found := strings.Cut(proposal.URL, "proposal-")
	if !found {
		t.Fatal("the proposal link carries no capability")
	}
	start := nextMonday().Add(2 * time.Hour)
	accept := AnyMap{"start": start, "end": start.Add(30 * time.Minute), "consent": AnyMap{"policy_version": "2026-01", "wording": "Contact me about this meeting."}}
	var accepted struct {
		ID string `json:"id"`
	}
	if status := publicCall(t, e, "POST", "/v1/public/proposal/"+token, accept, nil, &accepted); status != http.StatusAccepted {
		t.Fatalf("proposal acceptance → %d", status)
	}
	deliver()
	if !strings.Contains(deliveredDescription(t, provider, accepted.ID), hostAgenda) {
		t.Fatal("the invitation dropped the description its host wrote")
	}
}

// assertInvitationOmitsAgenda reads the last invitation body the host's calendar
// was sent. The management link arrives, the agenda does not, and no blank line
// stands where it was.
func assertInvitationOmitsAgenda(t *testing.T, provider *bookingProviderTransport, booked bookedInvitation, agenda string) {
	t.Helper()
	description := deliveredDescription(t, provider, booked.Invitation.ID)
	if strings.Contains(description, agenda) {
		t.Fatal("the host's calendar sent the booker's agenda to the address they typed")
	}
	if !strings.Contains(description, booked.Invitation.Token) {
		t.Fatal("the invitation no longer carries the guest's management link")
	}
	if strings.HasPrefix(description, "\n") {
		t.Fatal("the invitation description opens with a blank line where the agenda was")
	}
}

// deliveredDescription is the description field of the event the host's calendar
// was last sent for one invitation.
func deliveredDescription(t *testing.T, provider *bookingProviderTransport, invitationID string) string {
	t.Helper()
	var event struct {
		Description string `json:"description"`
	}
	if err := json.Unmarshal(provider.deliveredEvent(t, invitationID), &event); err != nil {
		t.Fatal(err)
	}
	return event.Description
}

// startMeetingDelivery boots the booking worker and answers a function running one
// calendar delivery pass to completion.
func startMeetingDelivery(t *testing.T, e *apptest.AppEnv) func() {
	t.Helper()
	runner, completed, failed := startBookingWorker(t, e)
	awaitPass := func() {
		t.Helper()
		ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
		defer cancel()
		if !jobtest.AwaitKindOutcome(ctx, t, completed, failed, "meeting_delivery") {
			t.Fatal("calendar delivery worker failed")
		}
	}
	// The worker runs one pass on start; settling it first keeps its outcome from
	// answering for a pass enqueued after the booking.
	awaitPass()
	return func() {
		t.Helper()
		if err := runner.Enqueue(context.Background(), compose.MeetingDeliveryArgs{}, nil); err != nil {
			t.Fatal(err)
		}
		awaitPass()
	}
}
