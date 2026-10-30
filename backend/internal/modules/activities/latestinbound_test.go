// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

var justInNoon = time.Date(2026, 10, 28, 12, 0, 0, 0, time.UTC)

func justInRow(kind crmcontracts.ActivityKind, hoursAgo int) crmcontracts.Activity {
	return crmcontracts.Activity{
		Id: openapi_types.UUID{byte(hoursAgo), byte(len(kind))}, Kind: kind,
		OccurredAt: justInNoon.Add(-time.Duration(hoursAgo) * time.Hour),
	}
}

func webFormNote(hoursAgo int) crmcontracts.Activity {
	row := justInRow(crmcontracts.ActivityKindNote, hoursAgo)
	row.SourceSystem = new(provenance.WebFormSource)
	return row
}

func withDirection(row crmcontracts.Activity, d crmcontracts.ActivityDirection) crmcontracts.Activity {
	row.Direction = &d
	return row
}

func withMeeting(row crmcontracts.Activity, s crmcontracts.ActivityMeetingStatus) crmcontracts.Activity {
	row.MeetingStatus = &s
	return row
}

func justInKinds(items []crmcontracts.LatestInbound) []crmcontracts.LatestInboundKind {
	kinds := make([]crmcontracts.LatestInboundKind, len(items))
	for i, item := range items {
		kinds[i] = item.Kind
	}
	return kinds
}

func TestUnansweredInboundMergesWaitingMailWithOpenFormsNewestFirst(t *testing.T) {
	mail := justInRow(crmcontracts.ActivityKindEmail, 5)
	form := webFormNote(2)
	got := unansweredInbound([]crmcontracts.Activity{mail}, []crmcontracts.Activity{form, mail})
	want := []crmcontracts.LatestInboundKind{crmcontracts.LatestInboundKindWebForm, crmcontracts.LatestInboundKindEmail}
	if kinds := justInKinds(got); len(kinds) != 2 || kinds[0] != want[0] || kinds[1] != want[1] {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
}

func TestUnansweredInboundDropsAFormAnsweredLater(t *testing.T) {
	cases := map[string]crmcontracts.Activity{
		"sent mail":    withDirection(justInRow(crmcontracts.ActivityKindEmail, 1), crmcontracts.ActivityDirectionOutbound),
		"sent message": withDirection(justInRow(crmcontracts.ActivityKindMessage, 1), crmcontracts.ActivityDirectionOutbound),
		"call":         justInRow(crmcontracts.ActivityKindCall, 1),
		"held meeting": withMeeting(justInRow(crmcontracts.ActivityKindMeeting, 1), crmcontracts.ActivityMeetingStatusHeld),
	}
	for name, answer := range cases {
		t.Run(name, func(t *testing.T) {
			if got := unansweredInbound(nil, []crmcontracts.Activity{answer, webFormNote(3)}); len(got) != 0 {
				t.Fatalf("got %v, want the form answered", justInKinds(got))
			}
		})
	}
}

func TestUnansweredInboundKeepsAFormWhatFollowedDidNotAnswer(t *testing.T) {
	cases := map[string]crmcontracts.Activity{
		"their reply":       withDirection(justInRow(crmcontracts.ActivityKindEmail, 1), crmcontracts.ActivityDirectionInbound),
		"our own note":      justInRow(crmcontracts.ActivityKindNote, 1),
		"canceled meeting":  withMeeting(justInRow(crmcontracts.ActivityKindMeeting, 1), crmcontracts.ActivityMeetingStatusCanceled),
		"no-show meeting":   withMeeting(justInRow(crmcontracts.ActivityKindMeeting, 1), crmcontracts.ActivityMeetingStatusNoShow),
		"call before it":    justInRow(crmcontracts.ActivityKindCall, 4),
		"sent mail earlier": withDirection(justInRow(crmcontracts.ActivityKindEmail, 6), crmcontracts.ActivityDirectionOutbound),
	}
	for name, other := range cases {
		t.Run(name, func(t *testing.T) {
			got := unansweredInbound(nil, []crmcontracts.Activity{other, webFormNote(3)})
			if len(got) != 1 || got[0].Kind != crmcontracts.LatestInboundKindWebForm {
				t.Fatalf("got %v, want the form still open", justInKinds(got))
			}
		})
	}
}

func TestUnansweredInboundIgnoresWaitingRowsThatAreNotMail(t *testing.T) {
	got := unansweredInbound([]crmcontracts.Activity{justInRow(crmcontracts.ActivityKindCall, 1)}, nil)
	if len(got) != 0 {
		t.Fatalf("got %v, want nothing", justInKinds(got))
	}
	if got == nil {
		t.Fatal("an empty answer must be an empty list, which the contract types as an array")
	}
}

func TestUnansweredInboundKeepsTheNewestTen(t *testing.T) {
	var waiting []crmcontracts.Activity
	for hours := 1; hours <= latestInboundLimit+3; hours++ {
		waiting = append(waiting, justInRow(crmcontracts.ActivityKindEmail, hours))
	}
	got := unansweredInbound(waiting, nil)
	if len(got) != latestInboundLimit {
		t.Fatalf("len = %d, want %d", len(got), latestInboundLimit)
	}
	if !got[0].Activity.OccurredAt.Equal(justInNoon.Add(-time.Hour)) {
		t.Fatalf("first = %v, want the newest", got[0].Activity.OccurredAt)
	}
}

func TestWhoseMoveFollowsTheMailBadgeThenTheDirection(t *testing.T) {
	badge := func(m crmcontracts.EmailSummaryMove) *crmcontracts.EmailSummaryMove { return &m }
	out, in := string(crmcontracts.ActivityDirectionOutbound), string(crmcontracts.ActivityDirectionInbound)
	cases := []struct {
		name      string
		move      *crmcontracts.EmailSummaryMove
		direction string
		want      crmcontracts.LastExchangeWhoseMove
	}{
		{"a mail we wait on", badge(crmcontracts.EmailSummaryMoveWaitingForThem), in, crmcontracts.LastExchangeWhoseMoveThem},
		{"a mail that needs our reply", badge(crmcontracts.EmailSummaryMoveNeedsReply), out, crmcontracts.LastExchangeWhoseMoveUs},
		{"a mail that settles the thread", badge(crmcontracts.EmailSummaryMoveNone), in, crmcontracts.LastExchangeWhoseMoveNeither},
		{"a call we made", nil, out, crmcontracts.LastExchangeWhoseMoveThem},
		{"a call they made", nil, in, crmcontracts.LastExchangeWhoseMoveUs},
		{"a meeting", nil, "", crmcontracts.LastExchangeWhoseMoveNeither},
	}
	for _, c := range cases {
		if got := WhoseMove(c.move, c.direction); got != c.want {
			t.Errorf("%s: whose move = %s, want %s", c.name, got, c.want)
		}
	}
}
