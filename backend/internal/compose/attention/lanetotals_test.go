// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// What the badge beside a bounded lane says.

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// tasksDue builds n open tasks, all due before the bound.
func tasksDue(n int) []Task {
	due := readInstant.Add(time.Hour)
	out := make([]Task, 0, n)
	for range n {
		out = append(out, Task{ID: ids.NewV7(), Subject: "Owed", DueAt: &due})
	}
	return out
}

// promisesDue builds n open promises, all due before the bound.
func promisesDue(n int) []Commitment {
	due := readInstant.Add(time.Hour)
	out := make([]Commitment, 0, n)
	for range n {
		out = append(out, Commitment{ID: ids.NewV7(), Body: "I will send it", DueAt: due})
	}
	return out
}

// THE PLANNED BADGE IS THE TOTAL, not the page it sits above.
//
// The lane is capped at a dozen, so a reader with thirteen pieces of work saw
// twelve cards and a badge of twelve — and this lane offers no second page to
// reach the thirteenth by. The count now says thirteen, which is the reading
// needs_you has always had: tell them forty, then show the few worth a sitting.
func TestThePlannedBadgeCountsWhatThereIsNotWhatFits(t *testing.T) {
	tasks := &stubTasks{rows: tasksDue(plannedCap), total: plannedCap + 1}
	s := NewService(
		stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	day, err := s.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if len(day.Planned) != plannedCap {
		t.Fatalf("the lane carries %d cards, want the cap of %d", len(day.Planned), plannedCap)
	}
	if day.Counts.Planned != plannedCap+1 {
		t.Errorf("counts.planned = %d, want %d — the badge is how many they have, and this lane "+
			"has no second page to find the last one by", day.Counts.Planned, plannedCap+1)
	}
}

// AND THE COMMITMENTS BADGE READS THE SAME WAY, since the two lanes were made
// consistent with each other and both differed from needs_you.
func TestTheCommitmentsBadgeCountsWhatThereIsNotWhatFits(t *testing.T) {
	promises := &stubCommitments{rows: promisesDue(plannedCap), total: plannedCap + 5}
	s := NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, promises, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	day, err := s.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if day.Commitments == nil || len(*day.Commitments) != plannedCap {
		t.Fatalf("the lane carries %v, want the cap of %d", day.Commitments, plannedCap)
	}
	if day.Counts.Commitments == nil || *day.Counts.Commitments != plannedCap+5 {
		t.Errorf("counts.commitments = %v, want %d", day.Counts.Commitments, plannedCap+5)
	}
}

// A COUNT THAT FAILS FAILS THE LANE, rather than falling back to the page
// length: a badge quietly reporting the cap is the bug this replaced, and one
// that appears when the count breaks is the same bug with a rarer trigger.
func TestACountThatWillNotAnswerIsNotReplacedByThePageLength(t *testing.T) {
	tasks := &stubTasks{rows: tasksDue(3), countErr: context.DeadlineExceeded}
	s := NewService(
		stubApprovals{}, stubDuplicates{}, tasks, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)

	if _, err := s.Assemble(context.Background()); err == nil {
		t.Error("the day assembled with a count that failed, so the badge came from somewhere else")
	}
}
