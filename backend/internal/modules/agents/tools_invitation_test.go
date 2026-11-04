// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"context"
	"strings"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// fixedRoster answers the roster delivery would resolve, a true boundary here.
type fixedRoster []connector.CalendarAttendee

func (r fixedRoster) InvitationRoster(context.Context, crmcontracts.MeetingInvitationRequest) ([]connector.CalendarAttendee, error) {
	return r, nil
}

// The approver reads every recipient, and an address no consent check clears
// says so: the host typed it, and no record stands behind it. A host-named
// address that a hidden contact still holds is checked, so it is not marked.
func TestTheStagedInvitationMarksAnAddressNoConsentCheckClears(t *testing.T) {
	contact := ids.NewV7()
	start := time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)
	further := []openapi_types.Email{"stranger@elsewhere.test", "hidden@buyer.test", "rep@ours.test"}
	in := crmcontracts.MeetingInvitationRequest{
		ContactId: crmcontracts.Id(contact), AttendeeEmail: "buyer@buyer.test", AttendeeEmails: &further,
		Start: start, End: start.Add(30 * time.Minute), Subject: "Review",
	}
	roster := fixedRoster{
		{Address: "buyer@buyer.test", Basis: connector.AttendeeContact, ID: contact, AskConsent: true},
		{Address: "stranger@elsewhere.test", Basis: connector.AttendeeHostNamed},
		{Address: "hidden@buyer.test", Basis: connector.AttendeeHostNamed, AskConsent: true},
		{Address: "rep@ours.test", Basis: connector.AttendeeColleague, ID: ids.NewV7()},
	}

	staged, err := StageSubject(context.Background(), NewMeetingInvitationCall(&multiLinkProvider{}, nil, roster, in))
	if err != nil {
		t.Fatal(err)
	}
	want := "→ buyer@buyer.test, stranger@elsewhere.test (no consent check), hidden@buyer.test, rep@ours.test ·"
	if !strings.Contains(staged.Summary, want) {
		t.Errorf("staged %q, want it to carry %q", staged.Summary, want)
	}

	plain, err := StageSubject(context.Background(), NewMeetingInvitationCall(&multiLinkProvider{}, nil, nil, in))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(plain.Summary, "→ buyer@buyer.test, stranger@elsewhere.test, hidden@buyer.test, rep@ours.test ·") {
		t.Errorf("without a roster the summary reads %q, want the plain list", plain.Summary)
	}
}
