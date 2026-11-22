// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// A host with no calendar provider is told to connect one. The tool door reads
// the same store error as the REST door.
func TestReliableAvailabilityWithoutACalendarAsksForOne(t *testing.T) {
	f := newInvitationFixture(t)
	profile := defaultSchedulingProfile()
	profile.Provider = ""
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	host, _ := principal.Actor(f.ctx)
	_, _, err := f.store.ReliableAvailability(f.ctx, ids.From[ids.UserKind](host.UserID), monday(8), monday(18), 0)
	fault, ok := errors.AsType[apperrors.FieldFault](err)
	if !ok {
		t.Fatalf("store answered %v, want a field refusal both doors classify as 422", err)
	}
	if field, _, message := fault.FieldFault(); field != "provider" || !strings.Contains(message, "Connect a calendar") {
		t.Errorf("refusal names %q %q, want the provider and how to connect one", field, message)
	}
	if status, _ := reliableStatus(f, f.store); status != http.StatusUnprocessableEntity {
		t.Errorf("REST answered %d, want 422", status)
	}
}

// An installation with no calendar service is not the host's to fix, so the
// answer is the calendar-unavailable refusal, as for a booking page.
func TestReliableAvailabilityWithoutACalendarServiceIsNotTheHostsToFix(t *testing.T) {
	f := newInvitationFixture(t)
	status, body := reliableStatus(f, f.store.WithSchedulingCalendar(nil))
	if status != http.StatusBadGateway || !strings.Contains(body, connector.CalendarUnavailableCode) || strings.Contains(body, "Connect a calendar") {
		t.Fatalf("answered %d %s, want 502 calendar_unavailable", status, body)
	}
}

func reliableStatus(f *invitationFixture, store *Store) (int, string) {
	reliable := true
	rec := httptest.NewRecorder()
	Handlers{store: store}.GetAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
		crmcontracts.GetAvailabilityParams{From: monday(8), To: monday(18), Reliable: &reliable})
	return rec.Code, rec.Body.String()
}

func TestReliableAvailabilityOfAnotherUsersCalendarStaysForbidden(t *testing.T) {
	f := newInvitationFixture(t)
	other := openapi_types.UUID(ids.NewV7())
	reliable := true
	rec := httptest.NewRecorder()
	Handlers{store: f.store}.GetAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
		crmcontracts.GetAvailabilityParams{From: monday(8), To: monday(18), Reliable: &reliable, HostUserId: &other})
	if rec.Code != http.StatusForbidden {
		t.Fatalf("a colleague's calendar answered %d %s, want 403", rec.Code, rec.Body.String())
	}
}

// A proposal link belongs to a guest who cannot connect the host's calendar,
// so a host without one answers as a paused booking page does.
func TestProposalAvailabilityOfAHostWithoutACalendarAnswersNotFound(t *testing.T) {
	f := newInvitationFixture(t)
	link, err := f.store.CreateProposal(f.ctx, crmcontracts.MeetingProposalRequest{ContactId: f.request.ContactId, AttendeeEmail: f.request.AttendeeEmail, Subject: f.request.Subject, DurationMinutes: 30})
	if err != nil {
		t.Fatal(err)
	}
	profile := defaultSchedulingProfile()
	profile.Provider = ""
	if _, err := f.store.SaveSchedulingProfile(f.ctx, profile); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	Handlers{store: f.store}.GetPublicProposalAvailability(rec, httptest.NewRequest(http.MethodGet, "/availability", nil).WithContext(f.ctx),
		strings.Split(link.URL, "proposal-")[1], crmcontracts.GetPublicProposalAvailabilityParams{From: monday(8), To: monday(18)})
	body := strings.ToLower(rec.Body.String())
	if rec.Code != http.StatusNotFound || strings.Contains(body, "calendar") || strings.Contains(body, "provider") {
		t.Fatalf("the guest was answered %d %s, want a 404 that never names the calendar", rec.Code, rec.Body.String())
	}
}
