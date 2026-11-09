// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

// Ranking the interaction edges. This is the arithmetic behind "who should
// make the introduction", and it is worth its own test because the ordering
// was wrong once already: the read orders by LAST CONTACT, and a surface that
// promises warmest-first has to re-rank — otherwise a one-line reply yesterday
// outranks a year of correspondence.

import (
	"math"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// edgeAt is an edge whose newest interaction is the contact's own, when they
// ever wrote, so its recency counts from that moment.
func edgeAt(user ids.UUID, daysAgo float64, count, in, out int) InteractionEdge {
	last := time.Now().Add(-time.Duration(daysAgo * float64(24*time.Hour)))
	edge := InteractionEdge{
		UserID:      user,
		LastAt:      last,
		Count90d:    count,
		InCount90d:  in,
		OutCount90d: out,
	}
	if in > 0 {
		edge.LastInboundAt = &last
	}
	return edge
}

func TestRankingIsByWarmthNotRecency(t *testing.T) {
	now := time.Now()
	recentButThin := ids.NewV7()
	oldButDeep := ids.NewV7()

	edges := []InteractionEdge{
		// One reply yesterday. The most RECENT contact by a mile.
		edgeAt(recentButThin, 1, 1, 1, 0),
		// A fortnight since the last message, but a real two-way history.
		edgeAt(oldButDeep, 14, 20, 10, 10),
	}
	SortByStrength(edges, now)

	if edges[0].UserID != oldButDeep {
		t.Errorf("ranked the recent one-liner first; the question is who KNOWS them, "+
			"and last-contact order answers a different one (got %s, want %s)",
			edges[0].UserID, oldButDeep)
	}
}

func TestRankingIsDeterministicOnATie(t *testing.T) {
	now := time.Now()
	// Two identical edges: same recency, same volume, same balance. Only the
	// id can separate them, and it must — an unordered payload renders the
	// same account differently on two loads.
	a, b := ids.NewV7(), ids.NewV7()
	low, high := a, b
	if b.String() < a.String() {
		low, high = b, a
	}
	first := []InteractionEdge{edgeAt(high, 3, 8, 4, 4), edgeAt(low, 3, 8, 4, 4)}
	SortByStrength(first, now)
	if first[0].UserID != low {
		t.Errorf("tie broken as %s first; the lower id must win so the order is stable", first[0].UserID)
	}
}

func TestNeverHavingSpokenSortsLast(t *testing.T) {
	now := time.Now()
	known := ids.NewV7()
	// An edge with no interactions in the window still exists — the pair have
	// corresponded at some point — but it must not outrank a live
	// relationship.
	edges := []InteractionEdge{
		edgeAt(ids.NewV7(), 200, 0, 0, 0),
		edgeAt(known, 2, 12, 6, 6),
	}
	SortByStrength(edges, now)
	if edges[0].UserID != known {
		t.Error("a colleague with no recent interaction outranked one with a live relationship")
	}
}

func TestStrengthOfMatchesTheSharedArithmetic(t *testing.T) {
	// The edge scores through the same leaf the workspace-wide score uses.
	// If these ever diverge, two numbers on one screen disagree.
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -5)
	edge := InteractionEdge{LastAt: last, LastInboundAt: &last, Count90d: 12, InCount90d: 7, OutCount90d: 5}

	got := edge.StrengthOf(now)
	want := relstrength.Compute(relstrength.Inputs{
		LastTouch: &last, LastEngaged: &last, Count90d: 12, Inbound90d: 7, Outbound90d: 5,
	}, now)
	if got != want {
		t.Errorf("the edge scored %+v, the shared fold scored %+v", got, want)
	}
	// And it is the spec's worked example, so a drift in either shows here.
	if got.Strength != 47 {
		t.Errorf("strength = %d, want 47 — formulas-and-rules §4's worked example", got.Strength)
	}
}

// A colleague's own later send does not warm the edge: recency counts from
// the contact's last reply, or the pair's last meeting.
func TestEdgeRecencyIgnoresOurLaterSend(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	replied := now.AddDate(0, 0, -20)
	met := now.AddDate(0, 0, -10)
	late, lateOut := 1, 1
	edge := InteractionEdge{
		LastAt: now, LastInboundAt: &replied, LastOutbound: &now, Count90d: 12, InCount90d: 6, OutCount90d: 6,
		LateCount90d: &late, LateOutCount90d: &lateOut,
	}
	if got, want := edge.StrengthOf(now).Recency, math.Exp2(-20.0/relstrength.HalfLifeDays); math.Abs(got-want) > 1e-9 {
		t.Errorf("recency = %v, want %v from the reply 20 days ago", got, want)
	}
	edge.LastMetAt = &met
	if got, want := edge.StrengthOf(now).Recency, math.Exp2(-10.0/relstrength.HalfLifeDays); math.Abs(got-want) > 1e-9 {
		t.Errorf("recency = %v, want %v from the meeting 10 days ago", got, want)
	}
}

// A row folded before the projection knew when the contact took part scores
// by the earlier rule, from its newest interaction.
func TestAnEdgeFoldedBeforeTheRuleScoresAsBefore(t *testing.T) {
	now := time.Date(2026, 6, 4, 12, 0, 0, 0, time.UTC)
	last := now.AddDate(0, 0, -5)
	edge := InteractionEdge{LastAt: last, Count90d: 12, InCount90d: 7, OutCount90d: 5}
	want := relstrength.Compute(relstrength.Inputs{
		LastTouch: &last, LastEngaged: &last, Count90d: 12, Inbound90d: 7, Outbound90d: 5,
	}, now)
	if got := edge.StrengthOf(now); got != want || got.Strength != 47 {
		t.Errorf("an edge with no late counts scored %+v, want the earlier rule's %+v", got, want)
	}
}

// Our send after the contact's last reply counts toward neither frequency nor
// reciprocity on the edge.
func TestAnEdgesUnansweredSendDoesNotBalanceIt(t *testing.T) {
	now := time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC)
	engaged := now.AddDate(0, 0, -19)
	none, one := 0, 1
	before := InteractionEdge{
		LastAt: engaged, LastInboundAt: &engaged, Count90d: 19, InCount90d: 10, OutCount90d: 9,
		LateCount90d: &none, LateOutCount90d: &none,
	}
	after := InteractionEdge{
		LastAt: now, LastInboundAt: &engaged, Count90d: 20, InCount90d: 10, OutCount90d: 10,
		LateCount90d: &one, LateOutCount90d: &one,
	}
	if b, a := before.StrengthOf(now), after.StrengthOf(now); a != b {
		t.Errorf("an unanswered send moved the edge from %+v to %+v", b, a)
	}
}
