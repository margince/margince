// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package jobs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

// notNow stands in for a dependency's "not now": the AI budget stop or a
// blocked provider, which satisfy Postponement by the same method.
type notNow struct{ at time.Time }

func (e notNow) Error() string      { return "not now" }
func (e notNow) RetryAt() time.Time { return e.at }

// A postponement waits as a snooze, measured from the moment the fault path
// sees it, inside the reschedule bounds. It is logged as a wait, not a failure.
func TestAPostponementWaitsWithinTheRescheduleBounds(t *testing.T) {
	now := time.Date(2026, 11, 20, 9, 0, 0, 0, time.UTC)
	previous := postponementClock
	postponementClock = func() time.Time { return now }
	t.Cleanup(func() { postponementClock = previous })
	var logged bytes.Buffer
	restore := slog.Default()
	slog.SetDefault(slog.New(slog.NewTextHandler(&logged, nil)))
	t.Cleanup(func() { slog.SetDefault(restore) })

	for name, tc := range map[string]struct {
		at   time.Time
		want time.Duration
	}{
		"provider probed again soon": {now.Add(90 * time.Second), 90 * time.Second},
		"budget window next month":   {now.Add(11 * 24 * time.Hour), maxRescheduleDelay},
		"moment already past":        {now.Add(-time.Minute), minRescheduleDelay},
	} {
		err := FaultForKind(context.Background(), "k", fmt.Errorf("reading: %w", notNow{tc.at}))
		snooze, ok := errors.AsType[*river.JobSnoozeError](err)
		if !ok || snooze.Duration != tc.want {
			t.Errorf("%s: err = %v, want a snooze of %s", name, err, tc.want)
		}
	}
	if strings.Contains(logged.String(), "level=ERROR") || !strings.Contains(logged.String(), "level=WARN") {
		t.Errorf("a postponement must log as a WARN wait, never an ERROR: %q", logged.String())
	}
}

// A last try ends the job unless its error brings the job back.
func TestOnlyAFaultOnTheLastTryThatIsNoWaitIsExhausted(t *testing.T) {
	last := &rivertype.JobRow{Attempt: 5, MaxAttempts: 5}
	failed := errors.New("the database went away")
	for name, tc := range map[string]struct {
		row  *rivertype.JobRow
		err  error
		want bool
	}{
		"fault on the last try":       {last, failed, true},
		"fault with tries left":       {&rivertype.JobRow{Attempt: 2, MaxAttempts: 5}, failed, false},
		"snooze on the last try":      {last, river.JobSnooze(time.Minute), false},
		"postponement, last try":      {last, notNow{}, false},
		"success on the last try":     {last, nil, false},
		"a job River did not deliver": {nil, failed, false},
	} {
		if got := Exhausted(tc.row, tc.err); got != tc.want {
			t.Errorf("%s: Exhausted = %v, want %v", name, got, tc.want)
		}
	}
}
