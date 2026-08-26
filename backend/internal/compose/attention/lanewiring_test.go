// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
)

// Each optional lane fills ITS OWN field. The three are wired through
// pointers-to-pointers into one response struct, which is exactly the shape
// where a copy-paste slip puts the meetings into at_risk and nothing fails to
// compile.
func TestEachOptionalLaneFillsItsOwnField(t *testing.T) {
	svc := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{}, stubBriefing{},
		&stubCommitments{rows: []Commitment{promise("a promise", readInstant)}},
		stubAtRisk{rows: []RiskyDeal{{Name: "a deal", QuietDays: 20}}},
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
