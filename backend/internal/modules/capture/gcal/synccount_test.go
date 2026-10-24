// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gcal

import (
	"context"
	"errors"
	"maps"
	"testing"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics/capturemetricstest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

var errWriteFault = errors.New("the database went away")

// faultingSink fails its first upsert and records the rest.
type faultingSink struct{ calls int }

func (s *faultingSink) Upsert(context.Context, connector.NormalizedRecord) (datasource.EntityRef, error) {
	s.calls++
	if s.calls == 1 {
		return datasource.EntityRef{}, errWriteFault
	}
	return datasource.EntityRef{}, nil
}

func twoMeetings(t *testing.T) *fakeAPI {
	t.Helper()
	return &fakeAPI{
		owner: gcalOwner, initialToken: "sync-abc",
		initial: [][]byte{
			eventJSON(t, "first", "confirmed", "Demo", "2026-07-16T09:00:00Z", gcalOwner, "client@acme.com"),
			eventJSON(t, "second", "confirmed", "Review", "2026-07-16T11:00:00Z", gcalOwner, "client@acme.com"),
		},
	}
}

// A write fault on the first of two listed events counts both as listed and the
// first as failed. The second is never attempted, so it has no outcome. The pull
// stops with the fault and advances no cursor, so the next pull retries both.
func TestAFailedWriteLeavesTheRestOfTheListUnattempted(t *testing.T) {
	listedBefore, before := capturemetricstest.CalendarCounts(t, connectorName)
	sink := &faultingSink{}
	cursor, err := New(fakeOAuth{access: "access-1"}, twoMeetings(t)).Sync(context.Background(), authBytes(t), nil, sink)
	if !errors.Is(err, errWriteFault) || cursor != nil {
		t.Fatalf("Sync = (%q, %v), want no cursor and the write fault", cursor, err)
	}
	if sink.calls != 1 {
		t.Errorf("sink called %d times, want 1: the pull stops at the fault", sink.calls)
	}
	listed, after := capturemetricstest.CalendarCounts(t, connectorName)
	if listed-listedBefore != 2 {
		t.Errorf("listed moved by %d, want 2", listed-listedBefore)
	}
	if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{"failed": 1}) {
		t.Errorf("outcomes moved %v, want failed by 1 and the second event unattempted", moved)
	}
}

// A replayed list counts again: each pull lists and settles what it is handed.
func TestAReplayedListIsCountedOnEachPull(t *testing.T) {
	listedBefore, before := capturemetricstest.CalendarCounts(t, connectorName)
	c := New(fakeOAuth{access: "access-1"}, twoMeetings(t))
	for range 2 {
		if _, err := c.Sync(context.Background(), authBytes(t), nil, &recordingSink{}); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	}
	listed, after := capturemetricstest.CalendarCounts(t, connectorName)
	if listed-listedBefore != 4 {
		t.Errorf("listed moved by %d, want 4", listed-listedBefore)
	}
	if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{"captured": 4}) {
		t.Errorf("outcomes moved %v, want captured by 4", moved)
	}
}

// An event past the capture edge is attempted as a move, which is its one
// outcome. A sink that cannot move is counted as such, not as a move.
func TestAnEventPastTheEdgeIsCountedAsItsMove(t *testing.T) {
	far := func() *fakeAPI {
		return &fakeAPI{owner: gcalOwner, initialToken: "sync-abc", initial: [][]byte{
			eventJSON(t, "pushed-out", "confirmed", "Offsite", "2028-06-01T10:00:00Z", gcalOwner, "client@acme.com"),
		}}
	}
	for want, sink := range map[string]connector.Sink{"beyond_horizon": &movingSink{}, "move_unsupported": &recordingSink{}} {
		_, before := capturemetricstest.CalendarCounts(t, connectorName)
		if _, err := pinnedConnector(far()).Sync(context.Background(), authBytes(t), nil, sink); err != nil {
			t.Fatalf("%s: Sync: %v", want, err)
		}
		_, after := capturemetricstest.CalendarCounts(t, connectorName)
		if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{want: 1}) {
			t.Errorf("outcomes moved %v, want %s by 1", moved, want)
		}
	}
}
