// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package graphcal

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

// A write fault on the first of two listed events counts both as listed and the
// first as failed. The second is never attempted, so it has no outcome. The pull
// stops with the fault and advances no cursor.
func TestAFailedWriteLeavesTheRestOfTheListUnattempted(t *testing.T) {
	first := eventJSON(t, "evt-1", "Kickoff", "2026-07-16T09:00:00.0000000", "UTC", owner, "client@acme.com")
	second := eventJSON(t, "evt-2", "Review", "2026-07-16T10:00:00.0000000", "UTC", owner, "client@acme.com")
	api := &fakeAPI{email: owner, initEvents: [][]byte{first, second}, initDelta: "https://graph/delta?$1"}
	listedBefore, before := capturemetricstest.CalendarCounts(t, connectorName)
	sink := &faultingSink{}
	cursor, err := newAt(&fakeOAuth{access: "a"}, api, fixedNow).Sync(context.Background(), sealedAuth(t), nil, sink)
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

func TestAReplayedListIsCountedOnEachPull(t *testing.T) {
	first := eventJSON(t, "evt-1", "Kickoff", "2026-07-16T09:00:00.0000000", "UTC", owner, "client@acme.com")
	api := &fakeAPI{email: owner, initEvents: [][]byte{first}, initDelta: "https://graph/delta?$1"}
	listedBefore, before := capturemetricstest.CalendarCounts(t, connectorName)
	c := newAt(&fakeOAuth{access: "a"}, api, fixedNow)
	for range 2 {
		if _, err := c.Sync(context.Background(), sealedAuth(t), nil, &recordingSink{}); err != nil {
			t.Fatalf("Sync: %v", err)
		}
	}
	listed, after := capturemetricstest.CalendarCounts(t, connectorName)
	if listed-listedBefore != 2 {
		t.Errorf("listed moved by %d, want 2", listed-listedBefore)
	}
	if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{"captured": 2}) {
		t.Errorf("outcomes moved %v, want captured by 2", moved)
	}
}
