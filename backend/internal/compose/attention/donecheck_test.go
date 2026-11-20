// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readsSeen is a snapshot that records each composed read it opens, and how
// many degradable lanes ran inside one.
type readsSeen struct {
	reads    *[]string
	inside   *bool
	budgeted *int
}

func (r readsSeen) InSnapshot(ctx context.Context, read string, fn func(context.Context) error) error {
	*r.reads = append(*r.reads, read)
	*r.inside = true
	defer func() { *r.inside = false }()
	return fn(ctx)
}

func (r readsSeen) Detached(ctx context.Context) context.Context { return ctx }

func (r readsSeen) Degradable(ctx context.Context, _ time.Duration, fn func(context.Context) error) error {
	if *r.inside {
		*r.budgeted++
	}
	return fn(ctx)
}

// doneCheckService is a day holding one meeting and a quiet relationship.
func doneCheckService(meeting ids.UUID, decay *stubDecay) *Service {
	kickOff := Meeting{ID: meeting, Subject: "Kick-off", StartsAt: readInstant.Add(30 * time.Minute), Preparation: ownPreparation(true)}
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, decay, &stubMeetings{rows: []Meeting{kickOff}},
		nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

// A Done check reads the day in a snapshot of its own (#11272). Outside one,
// no lane gets its statement budget, and one slow lane ran out the request.
func TestADoneCheckReadsTheDayInASnapshot(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	var reads []string
	var inside bool
	budgeted := 0
	spy := &doneStoreSpy{}
	svc := doneCheckService(meeting, &stubDecay{}).WithDone(spy).
		WithSnapshots(readsSeen{reads: &reads, inside: &inside, budgeted: &budgeted})

	err := svc.MarkRowsDone(meetingPrepReader(), []RowRef{{Source: sourceMeeting, RowID: meeting.String()}})
	if err != nil {
		t.Fatalf("marking a meeting the day draws: %v", err)
	}
	if len(spy.marked) != 1 {
		t.Fatalf("wrote %d batches, want 1", len(spy.marked))
	}
	if len(reads) == 0 || !slices.Contains(reads, ReadWorklistDone) {
		t.Fatalf("the check opened reads %v, want %q among them", reads, ReadWorklistDone)
	}
	if budgeted == 0 {
		t.Fatal("no lane of the check ran on a statement budget")
	}
}

// A Done check never reads the relationship decay lane. No decay row offers
// Done, so the lane cannot hold a row the check looks for, and it is the
// costliest lane a day has.
func TestADoneCheckDoesNotReadRelationshipDecay(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	quiet := []QuietRelationship{{ContactID: ids.NewV7(), Name: "a contact", QuietDays: 63, LastAt: readInstant}}

	shown := &stubDecay{rows: quiet}
	if _, err := doneCheckService(meeting, shown).Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, ""); err != nil {
		t.Fatalf("reading the Worklist: %v", err)
	}
	if shown.calls == 0 {
		t.Fatal("the Worklist itself must read the decay lane, or this test proves nothing")
	}

	checked := &stubDecay{rows: quiet}
	spy := &doneStoreSpy{}
	err := doneCheckService(meeting, checked).WithDone(spy).
		MarkRowsDone(meetingPrepReader(), []RowRef{{Source: sourceMeeting, RowID: meeting.String()}})
	if err != nil {
		t.Fatalf("marking a meeting the day draws: %v", err)
	}
	if checked.calls != 0 {
		t.Fatalf("the Done check read the decay lane %d times, want none", checked.calls)
	}
}
