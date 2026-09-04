// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// meeting_status pairs with kind meeting and nothing else. The forward case
// matters because the lead status ladder reads booked/held as engagement —
// a mapping that dropped the field would leave a hand-logged meeting unable
// to move a lead while a synced one does. The refusal matters because the
// database CHECK constrains only the vocabulary, not the pairing: without it
// a note carrying `held` stores silently as a meeting-shaped fact.

import (
	"errors"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestLogActivityInputCarriesAMeetingsStatus(t *testing.T) {
	status := crmcontracts.CreateActivityRequestMeetingStatusHeld

	in, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:          crmcontracts.CreateActivityRequestKindMeeting,
		MeetingStatus: &status,
		Source:        "human",
	})
	if err != nil {
		t.Fatalf("mapping a held meeting: %v", err)
	}
	if in.MeetingStatus == nil || *in.MeetingStatus != "held" {
		t.Fatalf("MeetingStatus = %v, want held — dropped, a hand-logged meeting cannot earn a lead's engaged step", in.MeetingStatus)
	}
}

func TestLogActivityInputRefusesAMeetingStatusOnANote(t *testing.T) {
	status := crmcontracts.CreateActivityRequestMeetingStatusHeld

	_, err := LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind:          crmcontracts.CreateActivityRequestKindNote,
		MeetingStatus: &status,
		Source:        "human",
	})

	var fault *MeetingStatusKindError
	if !errors.As(err, &fault) {
		t.Fatalf("err = %v, want a MeetingStatusKindError — a note carrying held must be a 422 naming the field", err)
	}
	if field, code, _ := fault.FieldFault(); field != "meeting_status" || code != faultNotValidForKind {
		t.Fatalf("FieldFault = (%q, %q), want (meeting_status, %s)", field, code, faultNotValidForKind)
	}
}

// The same field on the PATCH, which is where a human actually knows the
// answer: the meeting is over.
//
// The kind is NOT checked here, and that is the difference between the two
// mappings. A patch cannot change a kind, so the only honest thing to hold the
// field against is the kind the stored row carries — which the store reads
// under its own row lock. This mapping's job is to carry the value there
// without deciding anything.
func TestActivityUpdateInputCarriesAMeetingsStatus(t *testing.T) {
	status := crmcontracts.UpdateActivityRequestMeetingStatusNoShow

	in := activityUpdateInput(crmcontracts.UpdateActivityRequest{MeetingStatus: &status}, nil)

	if in.MeetingStatus == nil || *in.MeetingStatus != "no_show" {
		t.Fatalf("MeetingStatus = %v, want no_show — dropped here, the one moment a "+
			"human knows how the meeting went reaches no column", in.MeetingStatus)
	}
}

// And a patch that says nothing about it carries nothing, so the coalescing
// update leaves the stored value alone.
func TestActivityUpdateInputLeavesAnUnmentionedMeetingStatusAlone(t *testing.T) {
	subject := "Renamed"

	in := activityUpdateInput(crmcontracts.UpdateActivityRequest{Subject: &subject}, nil)

	if in.MeetingStatus != nil {
		t.Fatalf("a rename carried a meeting status of %q", *in.MeetingStatus)
	}
}
