// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"
)

// answering is the who-is-waiting seam reporting a window's figures, and
// recording the window it was asked for.
type answering struct {
	work AnsweredWork
	from *time.Time
	to   *time.Time
}

func (a *answering) Unanswered(context.Context, time.Time) ([]WaitingCustomer, bool, error) {
	return nil, false, nil
}

func (a *answering) Hidden(context.Context, time.Time) (HiddenWork, error) {
	return HiddenWork{}, nil
}

func (a *answering) Answered(
	_ context.Context, from, to time.Time,
) (AnsweredWork, error) {
	a.from, a.to = &from, &to
	return a.work, nil
}

// The window a caller names is the window the read gets.
//
// A projection that ignored the parameter would answer the default for every
// request, so a reader asking about last week and a reader asking about last
// quarter would be shown the same figure and have no way to tell.
func TestTheWindowAskedForIsTheWindowRead(t *testing.T) {
	t.Parallel()

	seam := &answering{}
	svc := unboundService()
	svc.waiting = seam

	got, err := svc.ResponseMetrics(context.Background(), 30)
	if err != nil {
		t.Fatal(err)
	}

	if seam.from == nil || seam.to == nil {
		t.Fatal("the seam was never asked")
	}
	if days := seam.to.Sub(*seam.from).Hours() / 24; days < 29 || days > 31 {
		t.Fatalf("read a window of %.0f days, wanted the 30 asked for", days)
	}
	// The window travels to the READER too, or a figure arrives with no way to
	// know what span it describes.
	if !got.From.Equal(*seam.from) || !got.To.Equal(*seam.to) {
		t.Fatalf("the answer names %s–%s over a read of %s–%s",
			got.From, got.To, *seam.from, *seam.to)
	}
}

// A caller naming no window gets the default rather than a zero-length one.
//
// Zero days would make `from` and `to` the same instant, and every figure would
// come back zero — a workspace that answers nothing, reported over a window
// nothing could have happened in.
func TestNoWindowAskedForReadsTheDefaultFortnight(t *testing.T) {
	t.Parallel()

	seam := &answering{}
	svc := unboundService()
	svc.waiting = seam

	if _, err := svc.ResponseMetrics(context.Background(), 0); err != nil {
		t.Fatal(err)
	}

	if seam.from == nil {
		t.Fatal("the seam was never asked")
	}
	if days := seam.to.Sub(*seam.from).Hours() / 24; days < 13 || days > 15 {
		t.Fatalf("read a window of %.0f days, wanted the default fortnight", days)
	}
}

// Every figure crosses the seam. A projection that dropped one would publish a
// zero, and a zero here reads as a workspace answering instantly or judging
// nothing — both flattering, both wrong.
func TestEveryResponseFigureReachesTheWire(t *testing.T) {
	t.Parallel()

	svc := unboundService()
	svc.waiting = &answering{work: AnsweredWork{
		Answered: 40, MedianMinutes: 95, Disposed: 12, DisposedNotSales: 3,
	}}

	got, err := svc.ResponseMetrics(context.Background(), 14)
	if err != nil {
		t.Fatal(err)
	}

	for name, pair := range map[string][2]int{
		"answered":           {got.Answered, 40},
		"median_minutes":     {got.MedianMinutes, 95},
		"disposed":           {got.Disposed, 12},
		"disposed_not_sales": {got.DisposedNotSales, 3},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s reached the wire as %d, wanted %d", name, pair[0], pair[1])
		}
	}
}

// An installation with no mail stream has nothing to have answered slowly, so
// an empty window is the true answer rather than a refusal.
func TestAnUnboundWaitingLaneAnswersAnEmptyWindow(t *testing.T) {
	t.Parallel()

	got, err := unboundService().ResponseMetrics(context.Background(), 14)
	if err != nil {
		t.Fatalf("an unbound lane should answer, not refuse: %v", err)
	}

	if got.Answered != 0 || got.Disposed != 0 {
		t.Fatalf("a lane that never ran reported work: %+v", got)
	}
	// The window is still named, so a reader is not handed figures with no span.
	if !got.To.After(got.From) {
		t.Fatalf("the empty answer names no window: %s–%s", got.From, got.To)
	}
}
