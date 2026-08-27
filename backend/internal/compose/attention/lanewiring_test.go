// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Each optional lane fills ITS OWN field. The four are wired through
// pointers-to-pointers into one response struct, which is exactly the shape
// where a copy-paste slip puts the meetings into at_risk and nothing fails to
// compile.
func TestEachOptionalLaneFillsItsOwnField(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for _, lane := range []struct {
		name  string
		items *[]crmcontracts.AttentionItem
		want  string
	}{
		{"meetings", out.Meetings, "a meeting"},
		{"at_risk", out.AtRisk, "a deal"},
		{"relationship_decay", out.RelationshipDecay, "a contact"},
		{"commitments", out.Commitments, "a promise"},
	} {
		if lane.items == nil || len(*lane.items) != 1 {
			t.Fatalf("%s carries %v, want one item", lane.name, lane.items)
		}
		if got := (*lane.items)[0]; got.Title == nil || *got.Title != lane.want {
			t.Errorf("%s holds %q, want %q — a lane wrote into the wrong field",
				lane.name, stringOr(got.Title), lane.want)
		}
	}
}

// No lane advertises an action the surface cannot perform.
//
// AttentionRow renders controls for `complete` and `snooze` only. A card
// offering anything else is a promise to a client that nothing keeps — and a
// generated client is entitled to draw a control for whatever the server says
// it offers. The three optional lanes therefore send no action at all until the
// navigation they would need actually exists.
//
// Derived from the lanes rather than listed, so a fourth joins the check by
// existing.
func TestNoOptionalLaneOffersAnActionTheSurfaceCannotPerform(t *testing.T) {
	performable := map[crmcontracts.AttentionItemActions]bool{
		"complete": true, "snooze": true,
	}
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
		&stubDecay{rows: []QuietRelationship{{Name: "a contact", QuietDays: 63, LastAt: readInstant}}},
		&stubMeetings{rows: []Meeting{{Subject: "a meeting", StartsAt: readInstant}}},
		fixedClock,
	)
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	for name, lane := range map[string]*[]crmcontracts.AttentionItem{
		"meetings": out.Meetings, "at_risk": out.AtRisk, "commitments": out.Commitments,
		"relationship_decay": out.RelationshipDecay,
	} {
		if lane == nil {
			t.Fatalf("%s is absent, so this gate checked nothing", name)
		}
		for _, item := range *lane {
			for _, action := range item.Actions {
				if !performable[action] {
					t.Errorf("%s offers %q, which no control on this surface performs",
						name, action)
				}
			}
		}
	}
}
