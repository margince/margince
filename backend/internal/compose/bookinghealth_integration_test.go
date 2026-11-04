// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Home names a paused booking page to its host once the page was used. It says
// nothing about a page nobody used. Both hold through the feed's real wiring.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/approvals"
)

func bookingCards(t *testing.T, h *aliasHosts, ctx context.Context) []string {
	t.Helper()
	feed := newAttentionService(h.e.Pool, approvals.NewService(h.e.DB()), func() time.Time { return h.now })
	day, err := feed.Assemble(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if day.BookingHealth == nil {
		t.Fatalf("booking_health is absent (omitted %v, failed %v), want it read", day.LanesOmitted, day.LanesFailed)
	}
	var kinds []string
	for _, card := range *day.BookingHealth {
		if card.Kind != nil {
			kinds = append(kinds, *card.Kind)
		}
	}
	return kinds
}

// Both hosts' pages are switched on and name a calendar nobody connected, so
// both are paused. Only Ben copied his link.
func TestHomeNamesAPausedBookingPageOnlyOnceItWasUsed(t *testing.T) {
	h := newAliasHosts(t)
	if err := h.store.MarkBookingPageShared(h.ben); err != nil {
		t.Fatal(err)
	}
	if got := bookingCards(t, h, h.ben); len(got) != 1 || got[0] != "calendar_unavailable" {
		t.Fatalf("Ben's Home carries %v, want one calendar_unavailable card", got)
	}
	if got := bookingCards(t, h, h.ada); len(got) != 0 {
		t.Fatalf("Ada never used her page and her Home carries %v, want no card", got)
	}
}
