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
	got := unansweredInbound([]crmcontracts.Activity{mail}, []crmcontracts.Activity{form}, nil)
	want := []crmcontracts.LatestInboundKind{crmcontracts.LatestInboundKindWebForm, crmcontracts.LatestInboundKindEmail}
	if kinds := justInKinds(got); len(kinds) != 2 || kinds[0] != want[0] || kinds[1] != want[1] {
		t.Fatalf("kinds = %v, want %v", kinds, want)
	}
}

func TestUnansweredInboundKeepsOnlyTheFormsAfterTheNewestAnswer(t *testing.T) {
	answer := []crmcontracts.Activity{justInRow(crmcontracts.ActivityKindCall, 3)}
	got := unansweredInbound(nil, []crmcontracts.Activity{webFormNote(1), webFormNote(4)}, answer)
	if len(got) != 1 || !got[0].Activity.OccurredAt.Equal(webFormNote(1).OccurredAt) {
		t.Fatalf("got %+v, want only the form that came in after the call", got)
	}
}

func TestUnansweredInboundIgnoresWaitingRowsThatAreNotMail(t *testing.T) {
	got := unansweredInbound([]crmcontracts.Activity{justInRow(crmcontracts.ActivityKindCall, 1)}, nil, nil)
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
	got := unansweredInbound(waiting, nil, nil)
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
