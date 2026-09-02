// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"testing"
	"time"
)

// hidingWork is the who-is-waiting seam reporting a backlog behind the queue.
type hidingWork HiddenWork

func (h hidingWork) Unanswered(context.Context, time.Time) ([]WaitingCustomer, bool, error) {
	return nil, false, nil
}

func (h hidingWork) Hidden(context.Context, time.Time) (HiddenWork, error) {
	return HiddenWork(h), nil
}

// The real constructor with every lane unbound, which is what these cases vary
// from. Built through NewService rather than as a struct literal: a test that
// assembles its own Service proves nothing about the one production assembles,
// and this file is about a projection the constructor wires.
func unboundService() *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil,
		nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

// An installation with no mail stream has no waiting queue, so nothing can be
// hidden from it — and that is a true answer rather than a degraded one.
//
// The alternative, refusing, would put an error on a page that has nothing
// wrong with it: an operator who never bound the lane would be told their
// guardrail is broken every time it was read.
func TestAnUnboundWaitingLaneHasNoHiddenBacklog(t *testing.T) {
	t.Parallel()

	got, err := unboundService().HiddenBacklog(context.Background())
	if err != nil {
		t.Fatalf("an unbound lane should answer, not refuse: %v", err)
	}

	if !got.Clear {
		t.Fatalf("reported work hidden by a lane that never ran: %+v", got)
	}
}

// `clear` and the figures come from ONE struct, so a client cannot be told the
// backlog is clear over counts that say otherwise.
//
// The flag is what a check asserts on, which is exactly why it must not be able
// to disagree with the numbers beside it: a guardrail that reports success over
// its own evidence is worse than no guardrail.
func TestClearNeverDisagreesWithTheFiguresBesideIt(t *testing.T) {
	t.Parallel()

	svc := unboundService()
	svc.waiting = hidingWork{Shown: 4, NotSales: 2}

	got, err := svc.HiddenBacklog(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	if got.NotSales != 2 {
		t.Fatalf("carried %d judged not_sales, wanted the seam's 2", got.NotSales)
	}
	if got.Clear {
		t.Fatalf("called the backlog clear over %d hidden by a judgement", got.NotSales)
	}
}

// Every figure crosses the seam. A projection that dropped one would publish a
// zero — the under-reporting direction, where a broken read is indistinguishable
// from a healthy queue.
func TestEveryHiddenFigureReachesTheWire(t *testing.T) {
	t.Parallel()

	svc := unboundService()
	svc.waiting = hidingWork{Shown: 9, SetAside: 1, NotSales: 2, PastHorizon: 3, Unlinked: 4}

	got, err := svc.HiddenBacklog(context.Background())
	if err != nil {
		t.Fatal(err)
	}

	for name, pair := range map[string][2]int{
		"shown":        {got.Shown, 9},
		"set_aside":    {got.SetAside, 1},
		"not_sales":    {got.NotSales, 2},
		"past_horizon": {got.PastHorizon, 3},
		"unlinked":     {got.Unlinked, 4},
	} {
		if pair[0] != pair[1] {
			t.Fatalf("%s reached the wire as %d, wanted %d", name, pair[0], pair[1])
		}
	}
}
