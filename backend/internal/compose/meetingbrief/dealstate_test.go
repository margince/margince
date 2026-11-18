// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestAMeetingWithNoDealHasNoDealStateSection(t *testing.T) {
	in := fullInput()
	in.Deal = nil
	for _, section := range Deterministic(in) {
		if section.Kind == crmcontracts.MeetingBriefSectionKindDealState && len(section.Sentences) > 0 {
			t.Fatalf("a meeting on no deal says where the deal stands: %+v", section.Sentences)
		}
	}
}

func TestTheLastConversationIsNeverTheMeetingBeingPrepared(t *testing.T) {
	in := fullInput()
	upcoming := ActIn{ID: in.ActivityID, Kind: "meeting", Subject: "Q3 review", At: in.StartsAt}
	in.Recent = append([]ActIn{upcoming}, in.Recent...)
	state := sectionOf(t, Deterministic(in), crmcontracts.MeetingBriefSectionKindDealState)
	first := state.Sentences[0]
	if first.Evidence[0].EntityID != activityID || !strings.Contains(first.Text, "Re: security review") {
		t.Fatalf("the last conversation = %q citing %s, want the email before the meeting", first.Text, first.Evidence[0].EntityID)
	}
}
