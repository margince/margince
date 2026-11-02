// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	"encoding/json"
	"strings"
	"testing"
)

// The plan prompt tells the model which deal each message is filed under. A
// thread on another, lost deal then cannot pass for this meeting's reason.
func TestEveryPlanMessageSaysWhichDealItIsFiledUnder(t *testing.T) {
	in := fullInput()
	onThis, onLost, onNone := padID(1), padID(2), padID(3)
	lost := &FiledDeal{Name: "Deal A", State: "lost"}
	in.History = []HistoryIn{
		{ID: onThis, Kind: "email", Subject: "Discovery agenda", Direction: "inbound", At: at(2), OnDeal: true},
		{ID: onLost, Kind: "email", Subject: "Fleet telematics", Direction: "inbound", At: at(20), OtherDeal: lost},
		{ID: onNone, Kind: "email", Subject: "Holiday hours", Direction: "outbound", At: at(40)},
	}
	for _, row := range in.History {
		in.Excerpts = append(in.Excerpts, ExcerptIn{
			ActivityID: row.ID, Subject: row.Subject, Direction: row.Direction, At: row.At, Text: row.Subject + " body",
		})
	}
	type shownMessage struct {
		ActivityID string      `json:"activity_id"`
		OnThisDeal *bool       `json:"on_this_deal"`
		OtherDeal  *PromptDeal `json:"other_deal"`
	}
	var shown struct {
		Moments []struct {
			Messages []shownMessage `json:"messages"`
		} `json:"account_moments"`
	}
	if err := json.Unmarshal([]byte(encodePlanPrompt(planPromptOf(in, floorFor(in)))), &shown); err != nil {
		t.Fatalf("the plan prompt is not JSON: %v", err)
	}
	want := map[string]struct {
		onThis bool
		other  *PromptDeal
	}{
		onThis: {onThis: true},
		onLost: {other: &PromptDeal{Name: "Deal A", State: "lost"}},
		onNone: {},
	}
	seen := 0
	for _, moment := range shown.Moments {
		for _, message := range moment.Messages {
			expected, known := want[message.ActivityID]
			if !known {
				t.Fatalf("the prompt shows a message nobody planted: %s", message.ActivityID)
			}
			seen++
			if message.OnThisDeal == nil || *message.OnThisDeal != expected.onThis {
				t.Errorf("%s: on_this_deal = %v, want %v", message.ActivityID, message.OnThisDeal, expected.onThis)
			}
			if (message.OtherDeal == nil) != (expected.other == nil) ||
				(message.OtherDeal != nil && *message.OtherDeal != *expected.other) {
				t.Errorf("%s: other_deal = %+v, want %+v", message.ActivityID, message.OtherDeal, expected.other)
			}
		}
	}
	if seen != len(want) {
		t.Errorf("the prompt shows %d of the %d planted messages", seen, len(want))
	}
}

// Each history row is filed against the deal the brief settled on, which can
// be the contact's leading deal chosen after history was read.
func TestEachHistoryRowIsFiledAgainstTheBriefsDeal(t *testing.T) {
	this := FiledDeal{ID: padID(10), Name: "Deal B", State: "open"}
	lost := FiledDeal{ID: padID(11), Name: "Deal A", State: "lost"}
	in := Input{Deal: &DealIn{ID: this.ID, Name: this.Name}, History: []HistoryIn{
		{ID: "on another deal", Filed: []FiledDeal{lost}},
		{ID: "withheld", Withheld: true},
		{ID: "on both", Filed: []FiledDeal{this, lost}},
		{ID: "on the chosen deal", Filed: []FiledDeal{this}},
		{ID: "on no deal"},
	}}
	fileUnderDeal(&in)
	want := map[string]struct {
		onDeal bool
		other  *FiledDeal
	}{
		"on another deal":    {other: &lost},
		"withheld":           {},
		"on both":            {onDeal: true},
		"on the chosen deal": {onDeal: true},
		"on no deal":         {},
	}
	for _, row := range in.History {
		expected := want[row.ID]
		if row.OnDeal != expected.onDeal {
			t.Errorf("%s: on this deal = %v, want %v", row.ID, row.OnDeal, expected.onDeal)
		}
		if (row.OtherDeal == nil) != (expected.other == nil) ||
			(row.OtherDeal != nil && *row.OtherDeal != *expected.other) {
			t.Errorf("%s: other deal = %+v, want %+v", row.ID, row.OtherDeal, expected.other)
		}
	}
}

func TestThePlanSystemTellsTheModelTheObjectiveServesThisDeal(t *testing.T) {
	for _, phrase := range []string{`"on_this_deal"`, "serve this meeting's deal", "background only", `"lost" as lost`} {
		if !strings.Contains(planSystem, phrase) {
			t.Errorf("the plan prompt does not say %q", phrase)
		}
	}
}
