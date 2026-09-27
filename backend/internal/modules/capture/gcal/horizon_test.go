// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

// How far ahead a calendar is captured, and how an occurrence past that edge
// still arrives once it comes within reach.

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

var horizonNow = time.Date(2026, 9, 27, 10, 0, 0, 0, time.UTC)

func pinnedConnector(api *fakeAPI) *Connector {
	c := New(fakeOAuth{access: "access-1"}, api)
	c.now = func() time.Time { return horizonNow }
	return c
}

// A series with no end is expanded by Google for decades. Only the occurrences
// within a year are captured; one monthly meeting once landed 393 rows, the
// last of them in 2056.
func TestAnOccurrenceMoreThanAYearAheadIsNotCaptured(t *testing.T) {
	api := &fakeAPI{
		owner:        gcalOwner,
		initialToken: "sync-abc",
		initial: [][]byte{
			eventJSON(t, "next-month", "confirmed", "Consulting Monthly", "2026-10-27T10:00:00Z", gcalOwner, "client@acme.com"),
			eventJSON(t, "in-2056", "confirmed", "Consulting Monthly", "2056-08-30T10:00:00Z", gcalOwner, "client@acme.com"),
		},
	}
	sink := &recordingSink{}
	if _, err := pinnedConnector(api).Sync(context.Background(), authBytes(t), nil, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(sink.recs) != 1 || sink.recs[0].NaturalKey.SourceID != "next-month" {
		t.Fatalf("captured %+v, want only the occurrence within a year", sink.recs)
	}
}

// The edge applies to incremental pulls too: editing a series makes Google
// report every occurrence of it again, far ones included.
func TestAnIncrementalPullHoldsTheSameEdge(t *testing.T) {
	api := &fakeAPI{
		owner:      gcalOwner,
		deltaToken: "sync-next",
		delta: [][]byte{
			eventJSON(t, "in-2040", "confirmed", "Consulting Monthly", "2040-01-01T10:00:00Z", gcalOwner, "client@acme.com"),
		},
	}
	prior, _ := json.Marshal(cursorState{SyncToken: "sync-abc", ListedAt: horizonNow.Add(-24 * time.Hour)})
	sink := &recordingSink{}
	if _, err := pinnedConnector(api).Sync(context.Background(), authBytes(t), prior, sink); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(sink.recs) != 0 {
		t.Fatalf("captured %d records from 2040, want none", len(sink.recs))
	}
}

// A list older than a month is taken again, because an occurrence that was past
// the edge last time never changes and no incremental pull mentions it. What
// the old token still holds is read first: the full list reaches back only 90
// days, so a change to an older meeting would be lost with the token.
func TestAStaleListIsTakenAgainAndRedated(t *testing.T) {
	api := &fakeAPI{
		owner: gcalOwner, initialToken: "sync-fresh", deltaToken: "sync-next",
		delta: [][]byte{eventJSON(t, "old-change", "confirmed", "Review", "2026-05-01T10:00:00Z", gcalOwner, "client@acme.com")},
	}
	prior, _ := json.Marshal(cursorState{SyncToken: "sync-abc", ListedAt: horizonNow.Add(-relistAfter)})
	sink := &recordingSink{}
	cur, err := pinnedConnector(api).Sync(context.Background(), authBytes(t), prior, sink)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if api.initialCalls != 1 || api.incrementalCalls != 1 {
		t.Fatalf("stale list: initial=%d incremental=%d, want the token drained, then a re-list", api.initialCalls, api.incrementalCalls)
	}
	if len(sink.recs) != 1 || sink.recs[0].NaturalKey.SourceID != "old-change" {
		t.Fatalf("captured %+v, want the change the old token still held", sink.recs)
	}
	cs, _ := parseCursor(cur)
	if cs.SyncToken != "sync-fresh" || !cs.ListedAt.Equal(horizonNow) {
		t.Fatalf("cursor = %+v, want the fresh token listed now", cs)
	}
}

// A fresh list keeps its date across incremental pulls; refreshing it on every
// pull would mean it never goes stale and the edge never moves.
func TestAnIncrementalPullKeepsTheListDate(t *testing.T) {
	listed := horizonNow.Add(-10 * 24 * time.Hour)
	api := &fakeAPI{owner: gcalOwner, deltaToken: "sync-next"}
	prior, _ := json.Marshal(cursorState{SyncToken: "sync-abc", ListedAt: listed})
	cur, err := pinnedConnector(api).Sync(context.Background(), authBytes(t), prior, &recordingSink{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	cs, _ := parseCursor(cur)
	if api.initialCalls != 0 || cs.SyncToken != "sync-next" || !cs.ListedAt.Equal(listed) {
		t.Fatalf("initial=%d cursor=%+v, want an incremental pull keeping the list date %s", api.initialCalls, cs, listed)
	}
}

// A cursor written before the list date was recorded re-lists once.
func TestACursorWithoutAListDateRelistsOnce(t *testing.T) {
	api := &fakeAPI{owner: gcalOwner, initialToken: "sync-fresh"}
	prior, _ := json.Marshal(cursorState{SyncToken: "sync-abc"})
	cur, err := pinnedConnector(api).Sync(context.Background(), authBytes(t), prior, &recordingSink{})
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if cs, _ := parseCursor(cur); api.initialCalls != 1 || !cs.ListedAt.Equal(horizonNow) {
		t.Fatalf("initial=%d cursor=%+v, want one re-list dated now", api.initialCalls, cs)
	}
}

// A cancellation past the edge still goes through: it may close a meeting
// captured before the edge existed, and it writes no new row.
func TestACancellationPastTheEdgeIsNotHeldBack(t *testing.T) {
	far := horizonNow.Add(2 * captureForwards)
	cancelled := eventJSON(t, "far", calendarCanceled, "Offsite", far.Format(time.RFC3339), gcalOwner, "client@acme.com")
	if beyondHorizon(cancelled, gcalOwner, horizonNow.Add(captureForwards)) {
		t.Fatal("a cancellation past the edge was held back")
	}
	booked := eventJSON(t, "far", "confirmed", "Offsite", far.Format(time.RFC3339), gcalOwner, "client@acme.com")
	if !beyondHorizon(booked, gcalOwner, horizonNow.Add(captureForwards)) {
		t.Fatal("a booked meeting past the edge was not held back")
	}
}
