// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// What a meeting IS decides what a good plan for it looks like, so a
// misclassification is not a cosmetic error: it prepares a rep to negotiate in
// a room that wanted a conversation.
func TestTheMeetingKindIsReadFromWhatTheRecordsSay(t *testing.T) {
	for _, tc := range []struct {
		name       string
		shape      func(*Input)
		want       crmcontracts.MeetingPlanTypeValue
		confidence crmcontracts.MeetingPlanTier
	}{
		{
			name: "a typed subject and an agreeing deal stage is the confident case",
			shape: func(in *Input) {
				in.Subject = "Angebot besprechen"
				in.Deal.Stage = "Proposal"
			},
			want:       crmcontracts.MeetingPlanTypeCommercial,
			confidence: crmcontracts.MeetingPlanTierHigh,
		},
		{
			name: "a subject with nothing agreeing is believed, less strongly",
			shape: func(in *Input) {
				in.Subject = "Coffee with Lars"
				in.Deal = nil
			},
			want:       crmcontracts.MeetingPlanTypeRelationship,
			confidence: crmcontracts.MeetingPlanTierMedium,
		},
		{
			name: "a stage with no subject to confirm it is the weakest reading",
			shape: func(in *Input) {
				in.Subject = "Sync"
				in.Deal.Stage = "Negotiation"
			},
			want:       crmcontracts.MeetingPlanTypeCommercial,
			confidence: crmcontracts.MeetingPlanTierLow,
		},
		{
			name: "a German subject reads the same as its English sibling",
			shape: func(in *Input) {
				in.Subject = "Kickoff Retrofit"
				in.Deal = nil
			},
			want:       crmcontracts.MeetingPlanTypeDelivery,
			confidence: crmcontracts.MeetingPlanTierMedium,
		},
		{
			name: "work in flight with nothing being sold is a delivery meeting",
			shape: func(in *Input) {
				in.Subject = "Weekly"
				in.Deal = nil
				in.Project = &ProjectIn{ID: projectID, Name: "Retrofit", Key: "RET"}
			},
			want:       crmcontracts.MeetingPlanTypeDelivery,
			confidence: crmcontracts.MeetingPlanTierHigh,
		},
		{
			name: "nothing to read is unknown, which is an answer",
			shape: func(in *Input) {
				in.Subject = "Sync"
				in.Deal = nil
			},
			want:       crmcontracts.MeetingPlanTypeUnknown,
			confidence: crmcontracts.MeetingPlanTierLow,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			in := fullInput()
			tc.shape(&in)
			got := classifyMeeting(in)
			if got.Value != tc.want {
				t.Errorf("meeting kind = %q, want %q (signals %v)", got.Value, tc.want, got.Signals)
			}
			if got.Confidence != tc.confidence {
				t.Errorf("confidence = %q, want %q (signals %v)", got.Confidence, tc.confidence, got.Signals)
			}
		})
	}
}

// A subject naming two families must not depend on map iteration order for its
// answer: the table decides, and it decides the same way every time.
func TestAConflictingSubjectIsReadInAFixedOrder(t *testing.T) {
	in := fullInput()
	in.Subject = "Demo and pricing"
	in.Deal = nil
	for range 20 {
		if got := classifyMeeting(in); got.Value != crmcontracts.MeetingPlanTypeCommercial {
			t.Fatalf("meeting kind = %q, want the first family in the table (commercial)", got.Value)
		}
	}
}

// A room where everyone is new and nobody has met is a FIRST discovery, and
// the same subject with history behind it is a follow-up. The distinction
// changes the whole plan, so it is read rather than assumed.
func TestDiscoverySplitsOnWhetherTheRoomHasMet(t *testing.T) {
	in := fullInput()
	in.Subject = "Intro call"
	in.Deal = nil
	in.Attendees = []AttendeeIn{{PersonID: personID, FullName: "Ana Roth", FirstTime: true}}
	in.PriorMeetings = nil
	if got := classifyMeeting(in); got.Value != crmcontracts.MeetingPlanTypeFirstDiscovery {
		t.Errorf("a room that has never met = %q, want first_discovery", got.Value)
	}

	in.PriorMeetings = []PriorMeetingIn{{ID: activityID, Subject: "Last time", StartsAt: at(2)}}
	if got := classifyMeeting(in); got.Value != crmcontracts.MeetingPlanTypeFollowupDiscovery {
		t.Errorf("a room that has met = %q, want followup_discovery", got.Value)
	}
}
