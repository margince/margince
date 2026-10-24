// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/capture/capturemetrics/capturemetricstest"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// countProvider is the provider these counts are read under. No other test in
// the package counts under it.
const countProvider = "countcal"

var errWriteFault = errors.New("the database went away")

// answeringSink answers every write with err and implements every calendar
// verb, so one fixture reaches each write CaptureOne and MoveOne make.
type answeringSink struct{ err error }

func (s answeringSink) Upsert(context.Context, connector.NormalizedRecord) (datasource.EntityRef, error) {
	return datasource.EntityRef{}, s.err
}

func (s answeringSink) CancelMeeting(context.Context, connector.NaturalKey, time.Time) error {
	return s.err
}

func (s answeringSink) MoveMeeting(context.Context, connector.NaturalKey, time.Time, *int) error {
	return s.err
}

// upsertOnly hides every verb but Upsert.
type upsertOnly struct{ connector.Sink }

var errSkipped = fmt.Errorf("internal only: %w", connector.ErrSkip)

// Every attempted event ends on one outcome, and a write fault both counts
// and stops the pull. CaptureOne and MoveOne are asked the same way.
func TestEveryAttemptedEventEndsOnOneOutcome(t *testing.T) {
	outside := []Actor{{Email: owner}, {Email: "client@acme.test"}}
	live := Event{ID: "e1", Attendees: outside}
	cancelled := Event{ID: "e2", Cancelled: true, Attendees: outside}
	capture := func(sink connector.Sink, ev Event) func() error {
		return func() error {
			return CaptureOne(context.Background(), rawEvent(t, ev), sink, owner, countProvider, testDecode)
		}
	}
	move := func(sink connector.Sink, ev Event) func() error {
		return func() error {
			return MoveOne(context.Background(), rawEvent(t, ev), sink, owner, countProvider, testDecode)
		}
	}
	cases := map[string]struct {
		run     func() error
		wantErr bool
	}{
		"captured":                           {run: capture(answeringSink{}, live)},
		"sink_skipped":                       {run: capture(answeringSink{err: errSkipped}, live)},
		"failed":                             {run: capture(answeringSink{err: errWriteFault}, live), wantErr: true},
		"dropped_no_event_id":                {run: capture(answeringSink{}, Event{Attendees: outside})},
		"dropped_no_party_besides_the_owner": {run: capture(answeringSink{}, Event{ID: "e3", Attendees: outside[:1]})},
		"cancelled":                          {run: capture(answeringSink{}, cancelled)},
		"declined_by_the_calendar_owner":     {run: capture(answeringSink{}, Event{ID: "e4", OwnerDeclined: true, Attendees: outside})},
		"cancel_unsupported":                 {run: capture(upsertOnly{answeringSink{}}, cancelled)},
		"beyond_horizon":                     {run: move(answeringSink{}, live)},
		"move_unsupported":                   {run: move(upsertOnly{answeringSink{}}, live)},
	}
	for want, tc := range cases {
		_, before := capturemetricstest.CalendarCounts(t, countProvider)
		err := tc.run()
		if (err != nil) != tc.wantErr {
			t.Errorf("%s: error %v, want error %v", want, err, tc.wantErr)
		}
		_, after := capturemetricstest.CalendarCounts(t, countProvider)
		if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{want: 1}) {
			t.Errorf("%s: counts moved %v, want only %s by 1", want, moved, want)
		}
	}
}

// A cancellation or a move that fails or is skipped is counted the way an
// upsert is. An event the decoder cannot read is counted with no id.
func TestAFailedCancelMoveOrReadEndsOnOneOutcome(t *testing.T) {
	outside := []Actor{{Email: owner}, {Email: "client@acme.test"}}
	cancelled := rawEvent(t, Event{ID: "e5", Cancelled: true, Attendees: outside})
	live := rawEvent(t, Event{ID: "e6", Attendees: outside})
	noID := rawEvent(t, Event{Attendees: outside})
	cases := []struct {
		name, want string
		run        func() error
		wantErr    bool
	}{
		{"cancel skipped", "sink_skipped", func() error {
			return CaptureOne(context.Background(), cancelled, answeringSink{err: errSkipped}, owner, countProvider, testDecode)
		}, false},
		{"cancel failed", "failed", func() error {
			return CaptureOne(context.Background(), cancelled, answeringSink{err: errWriteFault}, owner, countProvider, testDecode)
		}, true},
		{"move skipped", "sink_skipped", func() error {
			return MoveOne(context.Background(), live, answeringSink{err: errSkipped}, owner, countProvider, testDecode)
		}, false},
		{"move failed", "failed", func() error {
			return MoveOne(context.Background(), live, answeringSink{err: errWriteFault}, owner, countProvider, testDecode)
		}, true},
		{"move without an id", "dropped_no_event_id", func() error {
			return MoveOne(context.Background(), noID, answeringSink{}, owner, countProvider, testDecode)
		}, false},
		{"capture unreadable", "unreadable", func() error {
			return CaptureOne(context.Background(), []byte("{not json"), answeringSink{}, owner, countProvider, testDecode)
		}, false},
		{"move unreadable", "unreadable", func() error {
			return MoveOne(context.Background(), []byte("{not json"), answeringSink{}, owner, countProvider, testDecode)
		}, false},
	}
	for _, tc := range cases {
		_, before := capturemetricstest.CalendarCounts(t, countProvider)
		err := tc.run()
		if (err != nil) != tc.wantErr || tc.wantErr && !errors.Is(err, errWriteFault) {
			t.Errorf("%s: error %v, want the write fault returned: %v", tc.name, err, tc.wantErr)
		}
		_, after := capturemetricstest.CalendarCounts(t, countProvider)
		if moved := capturemetricstest.Moved(before, after); !maps.Equal(moved, map[string]int{tc.want: 1}) {
			t.Errorf("%s: counts moved %v, want only %s by 1", tc.name, moved, tc.want)
		}
	}
}
