// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

// How much of the timeline the brief is shown.
//
// Its own file because the question is one the other input tests do not ask:
// they pin what each folded row CARRIES, and this pins how many rows there are.
// The defect it guards is invisible to every one of them — a contact whose
// recent messages are all one exchange, where the window decides whether the
// model ever sees the topic the relationship is actually about.

import (
	"fmt"
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// timelineOf builds a view carrying n email rows, newest first, each naming its
// own position so a test can say WHICH rows survived the fold.
//
// Newest first because that is the order the 360 hands the timeline over in,
// and the fold takes the front of it: a fixture built the other way round would
// prove the opposite of what it claims.
func timelineOf(n int) crmcontracts.Contact360 {
	at := time.Date(2026, time.September, 16, 9, 0, 0, 0, time.UTC)
	available := crmcontracts.ActivityContentStateAvailable
	direction := crmcontracts.ActivityDirectionInbound
	var rows []crmcontracts.Activity
	for i := range n {
		id := openapi_types.UUID(ids.NewV7())
		subject := fmt.Sprintf("message %d", i+1)
		preview := fmt.Sprintf("the words of message %d", i+1)
		occurred := at.Add(-time.Duration(i) * time.Hour)
		rows = append(rows, crmcontracts.Activity{
			Id: id, Kind: crmcontracts.ActivityKindEmail, OccurredAt: occurred,
			Subject: &subject, Direction: &direction, ContentState: &available,
			EmailSummary: &crmcontracts.EmailSummary{
				ActivityId: id, OccurredAt: occurred, Subject: &subject, Preview: &preview,
				Move: crmcontracts.EmailSummaryMoveNone, DisplayStatus: crmcontracts.EmailAccessStatusTeam,
			},
		})
	}
	view := crmcontracts.Contact360{Contact: crmcontracts.Contact{FullName: "Anna Weber"}}
	view.Activities = &struct {
		Data []crmcontracts.Activity `json:"data"`
		Page crmcontracts.PageInfo   `json:"page"`
	}{Data: rows}
	return view
}

// The window reaches past the newest exchange.
//
// Six was the old bound, and on a contact whose last six messages were one
// scheduling thread it meant the model saw one topic and led with it — a booked
// meeting reported as the state of a relationship, while the substantive
// history sat in the seventh row. This asserts the rows that used to be cut are
// the ones the model is now shown.
func TestTheWindowReachesPastTheNewestExchange(t *testing.T) {
	t.Parallel()
	in := FromView(timelineOf(20))

	if len(in.Recent) != BriefInputActivities {
		t.Fatalf("the fold carried %d rows, want %d", len(in.Recent), BriefInputActivities)
	}
	// The seventh row is the first one the old bound cut, and the twelfth is the
	// last one this bound admits. Both by their own words rather than by count:
	// a length assertion passes over a fold that carried twelve copies of the
	// newest message.
	if got := in.Recent[6].Preview; got != "the words of message 7" {
		t.Errorf("the seventh row reads %q, want the message the old six-row window cut", got)
	}
	if got := in.Recent[BriefInputActivities-1].Preview; got != "the words of message 12" {
		t.Errorf("the last carried row reads %q, want the twelfth message", got)
	}
}

// And it stops there. The bound is what keeps the fingerprint from churning on
// activity that never changes the brief, so a fold that carried everything
// would rewrite every cached brief on every captured mail.
func TestTheWindowStopsAtItsBound(t *testing.T) {
	t.Parallel()
	in := FromView(timelineOf(BriefInputActivities + 5))

	if len(in.Recent) != BriefInputActivities {
		t.Fatalf("the fold carried %d rows, want the bound of %d", len(in.Recent), BriefInputActivities)
	}
	for _, row := range in.Recent {
		if row.Preview == fmt.Sprintf("the words of message %d", BriefInputActivities+1) {
			t.Error("a row past the bound reached the prompt")
		}
	}
}

// A contact with less history than the bound carries what it has, rather than
// the fold reaching for rows that are not there.
func TestAShortTimelineCarriesEveryRowItHas(t *testing.T) {
	t.Parallel()
	in := FromView(timelineOf(3))

	if len(in.Recent) != 3 {
		t.Fatalf("the fold carried %d rows for a three-message contact, want 3", len(in.Recent))
	}
}
