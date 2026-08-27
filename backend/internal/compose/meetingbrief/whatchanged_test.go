// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The section names its baseline, lists only what happened after it, and
// takes those claims out of the set so no later section repeats them.
func TestWhatChangedListsOnlyWhatHappenedAfterTheReaderLastSpoke(t *testing.T) {
	in := fullInput()
	in.LastSpokeAt = ptr(at(5))
	in.Commitments = []ClaimIn{
		{PersonName: "Ana Roth", Kind: kindObjection, Body: "the cure period", Status: statusOpen, SourceID: activityID, OccurredAt: ptr(at(7))},
		{PersonName: "Ana Roth", Kind: kindDecision, Body: "pilot first", Status: "done", SourceID: activityID, OccurredAt: ptr(at(2))},
	}
	in.Recent = []ActIn{
		{ID: activityID, Kind: "email", Subject: "Re: redline", Direction: "inbound", At: at(8)},
		{ID: activityID, Kind: "email", Subject: "older", Direction: "outbound", At: at(3)},
	}
	sections := Deterministic(in)
	changed := sectionOf(t, sections, crmcontracts.MeetingBriefSectionKindWhatChanged)
	texts := make([]string, 0, len(changed.Sentences))
	for _, line := range changed.Sentences {
		texts = append(texts, line.Text)
	}
	joined := strings.Join(texts, " | ")
	for _, want := range []string{"You last dealt with this room 5 days ago", "Since then Ana Roth objected: the cure period", "1 conversation since then, the latest \"Re: redline\""} {
		if !strings.Contains(joined, want) {
			t.Errorf("what changed = %q, want it to say %q", joined, want)
		}
	}
	if strings.Contains(joined, "pilot first") {
		t.Errorf("a decision from before the baseline is not a change: %q", joined)
	}
	risks := sectionOf(t, sections, crmcontracts.MeetingBriefSectionKindRisks)
	for _, line := range risks.Sentences {
		if strings.Contains(line.Text, "cure period") {
			t.Errorf("the objection what-changed took is said again as a risk: %q", line.Text)
		}
	}
}

func TestFirstContactIsSaidRatherThanNothingChanged(t *testing.T) {
	in := fullInput()
	in.LastSpokeAt = nil
	changed := sectionOf(t, Deterministic(in), crmcontracts.MeetingBriefSectionKindWhatChanged)
	if len(changed.Sentences) != 1 || !strings.HasPrefix(changed.Sentences[0].Text, "First contact") {
		t.Fatalf("what changed = %+v, want the first-contact line alone", changed.Sentences)
	}
}

func TestAQuietSpellSaysNothingCapturedHasChanged(t *testing.T) {
	in := fullInput()
	in.LastSpokeAt = ptr(at(9))
	in.Commitments = nil
	in.Recent = nil
	changed := sectionOf(t, Deterministic(in), crmcontracts.MeetingBriefSectionKindWhatChanged)
	if len(changed.Sentences) != 1 || !strings.Contains(changed.Sentences[0].Text, "Nothing captured has changed since") {
		t.Fatalf("what changed = %+v, want the baseline line saying nothing changed", changed.Sentences)
	}
}

// A deal that moved while nobody was talking is the thing a rep most needs to
// know walking in, and the section used to be silent about it while looking
// complete: it read claims and conversations only.
func TestWhatChangedNamesWhatHappenedToTheDealNotOnlyWhatWasSaid(t *testing.T) {
	in := fullInput()
	in.LastSpokeAt = ptr(at(5))
	in.Commitments = nil
	in.Recent = nil
	in.DealMoves = []DealMoveIn{
		{At: at(2), Text: "Since then offer Q-2026-014 revision 3 was sent.", DealID: dealID},
		{At: at(4), Text: "Since then the deal moved from Qualified to Proposal.", DealID: dealID},
		{At: at(1), Text: "Since then Rita Reviewer confirmed a document in the Deal Room.", DealID: dealID},
	}
	changed := sectionOf(t, Deterministic(in), crmcontracts.MeetingBriefSectionKindWhatChanged)
	var joined string
	for _, line := range changed.Sentences {
		joined += line.Text + " | "
	}
	for _, want := range []string{"revision 3 was sent", "moved from Qualified to Proposal", "confirmed a document in the Deal Room"} {
		if !strings.Contains(joined, want) {
			t.Errorf("what changed = %q, want it to say %q", joined, want)
		}
	}
	if strings.Contains(joined, "Nothing captured has changed") {
		t.Errorf("the section called a moved deal a quiet spell: %q", joined)
	}
}

// Each move cites the deal, because the deal page is where a reader goes to
// see a stage move or an offer — citing the meeting would send them nowhere.
func TestEveryDealMoveCitesTheDeal(t *testing.T) {
	in := fullInput()
	in.LastSpokeAt = ptr(at(5))
	in.Commitments = nil
	in.Recent = nil
	in.DealMoves = []DealMoveIn{{At: at(2), Text: "Since then the deal moved to Proposal.", DealID: dealID}}
	changed := sectionOf(t, Deterministic(in), crmcontracts.MeetingBriefSectionKindWhatChanged)
	for _, line := range changed.Sentences {
		if !strings.Contains(line.Text, "moved to Proposal") {
			continue
		}
		if len(line.Evidence) != 1 || line.Evidence[0].EntityType != citeDeal || line.Evidence[0].EntityID != dealID {
			t.Fatalf("the stage-move line cites %+v, want the deal %s", line.Evidence, dealID)
		}
		return
	}
	t.Fatal("the stage move was not rendered at all")
}

// A reader who cannot see Deal Rooms is TOLD so. Without this the brief reads
// exactly like a brief about a deal whose buyer has done nothing, and a rep
// would walk in believing the room was quiet.
func TestABriefBlindToTheDealRoomSaysSo(t *testing.T) {
	in := fullInput()
	in.RoomHidden = true
	got := omissions(in)
	if got == nil || len(*got) != 1 {
		t.Fatalf("omissions = %v, want the Deal Room named once", got)
	}
	if (*got)[0].Source != "deal_room" {
		t.Errorf("omission names source %q, want deal_room", (*got)[0].Source)
	}
	if !strings.Contains((*got)[0].Reason, "Deal Room") {
		t.Errorf("the reason %q does not say what is missing", (*got)[0].Reason)
	}
}

// A reader who can see everything gets no omissions at all, not an empty list:
// an empty array invites a client to draw an empty "what you cannot see"
// heading over nothing.
func TestABriefThatSawEverythingNamesNoOmission(t *testing.T) {
	in := fullInput()
	in.RoomHidden = false
	if got := omissions(in); got != nil {
		t.Fatalf("omissions = %v, want nil for a reader who saw everything", got)
	}
}
