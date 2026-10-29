// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// A meeting the reader has not confirmed ready says so. A meeting this reader
// does not attend or may not read says nothing at all.

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// meetingPrepReader is a human reading their own day. Without one the queue
// fails closed and every row disappears, which would make both tests below
// pass for the wrong reason.
func meetingPrepReader() context.Context {
	return principal.WithActor(context.Background(), principal.Principal{
		Type:        principal.PrincipalHuman,
		UserID:      ids.MustParse("01a05500-0000-7000-8000-000000000001"),
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}

func meetingPrepService(rows []Meeting) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, &stubMeetings{rows: rows},
		nil, nil, nil, nil, nil, nil, nil, nil, fixedClock)
}

// reasonsOn is the reason vocabulary one meeting row published.
func reasonsOn(t *testing.T, rows []crmcontracts.WorklistItem, subject string) []string {
	t.Helper()
	for _, row := range rows {
		if row.Title == nil || *row.Title != subject {
			continue
		}
		kinds := make([]string, 0, len(row.Because))
		for _, because := range row.Because {
			kinds = append(kinds, string(because.Kind))
		}
		return kinds
	}
	t.Fatalf("no row titled %q on the queue", subject)
	return nil
}

func TestAMeetingTheReaderHasNotConfirmedSaysSo(t *testing.T) {
	soon := readInstant.Add(30 * time.Minute)
	svc := meetingPrepService([]Meeting{
		{ID: ids.NewV7(), Subject: "unprepared", StartsAt: soon, Preparation: ownPreparation(false)},
		{ID: ids.NewV7(), Subject: "prepared", StartsAt: soon, Preparation: ownPreparation(true)},
	})

	day, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if got := reasonsOn(t, day.Queue, "unprepared"); !slices.Contains(got, "meeting_unprepared") {
		t.Errorf("a meeting the reader has not confirmed published %v, wanted meeting_unprepared among them", got)
	}
	if got := reasonsOn(t, day.Queue, "prepared"); slices.Contains(got, "meeting_unprepared") {
		t.Errorf("a prepared meeting published %v, which claims it is unprepared", got)
	}
}

// The refusal case. A meeting whose content is withheld, or that the reader
// does not attend, reaches the lane with no preparation. The queue must then
// say nothing rather than tell a reader to prepare it.
func TestAMeetingThisReaderCannotReadClaimsNothingAboutPreparation(t *testing.T) {
	svc := meetingPrepService([]Meeting{{
		ID: ids.NewV7(), Subject: "withheld", StartsAt: readInstant.Add(30 * time.Minute),
	}})

	day, err := svc.Worklist(meetingPrepReader(), "", "", ids.UUID{}, 25, "")
	if err != nil {
		t.Fatalf("worklist: %v", err)
	}

	if got := reasonsOn(t, day.Queue, "withheld"); slices.Contains(got, "meeting_unprepared") {
		t.Errorf("a withheld meeting published %v — an empty body it may not read is not an unprepared meeting", got)
	}
}

// The mark is written in one place and read in another, and nothing but this
// test makes them agree. A second spelling would not fail a build: the writer
// would stamp a word the reader never matches, and the reason would quietly
// stop appearing on the rows that earned it.
func TestTheUnpreparedMarkHasOneSpelling(t *testing.T) {
	written := meetingItem(Meeting{
		ID: ids.NewV7(), Subject: "unprepared", StartsAt: readInstant,
		Preparation: ownPreparation(false),
	})
	if written.Kind == nil {
		t.Fatal("meetingItem stamped no kind on a meeting it knows is unprepared")
	}
	row := classifyMeeting(written, readInstant)
	for _, because := range row.item.Because {
		if because.Kind == "meeting_unprepared" {
			return
		}
	}
	t.Errorf("classifyMeeting did not read the mark meetingItem wrote (%q) — the two spellings have drifted", *written.Kind)
}

// ownPreparation is the reader's own preparation for a meeting they attend.
func ownPreparation(ready bool) *crmcontracts.MeetingPreparation {
	state := crmcontracts.MeetingPreparationStateNeedsReview
	if ready {
		state = crmcontracts.MeetingPreparationStateReady
	}
	return &crmcontracts.MeetingPreparation{State: state, Revision: "r1", Changed: []crmcontracts.MeetingPreparationChanged{}}
}
