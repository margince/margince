// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"strings"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestTranscriptKeepsAnAlreadyFiledMeetingNextStep(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, subject string
		paddedSummary bool
		wantProposals int
	}{
		{name: "same work", subject: "  SEND   the revised pricing  "},
		{name: "padded model summary", subject: "Send the revised pricing", paddedSummary: true},
		{name: "different work", subject: "Book another review", wantProposals: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := setupTranscript(t)
			_, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
				Kind: "task", Subject: &tc.subject, Source: "manual", SourceActivityID: &e.activity.UUID,
			})
			if err != nil {
				t.Fatal(err)
			}
			reply := groundedReply(t, 3, 0.82, e.Rep1)
			if tc.paddedSummary {
				reply = strings.ReplaceAll(reply, `"Send the revised pricing"`, `"\tSEND   the revised pricing\n"`)
			}
			read := e.read(t, cannedBrain{reply: reply})
			if len(read.ProposalIDs) != tc.wantProposals {
				t.Fatalf("got %d proposals, want %d", len(read.ProposalIDs), tc.wantProposals)
			}
			if got := e.taskCount(t); got != 1 {
				t.Fatalf("filed task became %d tasks", got)
			}
		})
	}
}

func TestTranscriptDoesNotMistakeAnotherColleaguesTaskForTheNextStep(t *testing.T) {
	t.Parallel()
	e := setupTranscript(t)
	assignee := ids.From[ids.UserKind](e.Rep2)
	_, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "task", Subject: new("Send the revised pricing"), Source: "manual",
		SourceActivityID: &e.activity.UUID, AssigneeID: &assignee,
	})
	if err != nil {
		t.Fatal(err)
	}
	read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.82, e.Rep1)})
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("another colleague's task suppressed the proposal: %+v", read.ProposalIDs)
	}
}

func TestTranscriptCanBeReadAfterItsMeetingWasArchived(t *testing.T) {
	t.Parallel()
	e := setupTranscript(t)
	meeting, _, err := e.Activities.LogActivity(e.ctx, activities.LogActivityInput{
		Kind: "meeting", Subject: new("Quarterly review"), Source: "manual",
		OccurredAt: new(clocktest.Now(t).Add(-time.Hour)),
	})
	if err != nil {
		t.Fatal(err)
	}
	input, err := activities.LogActivityInputFrom(crmcontracts.CreateActivityRequest{
		Kind: "meeting", Subject: new("Review transcript"), Body: new(transcriptBody),
		Source: "manual", SourceSystem: new("transcript"), RecordsMeetingId: &meeting.Id,
	})
	if err != nil {
		t.Fatal(err)
	}
	transcript, _, err := e.Activities.LogActivity(e.ctx, input)
	if err != nil {
		t.Fatal(err)
	}
	e.activity = ids.From[ids.ActivityKind](ids.UUID(transcript.Id))
	meetingID := ids.From[ids.ActivityKind](ids.UUID(meeting.Id))
	if _, err := e.Activities.ArchiveActivity(e.Admin(), meetingID, nil); err != nil {
		t.Fatal(err)
	}
	read := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.82, e.Rep1)})
	if read.Status != activities.TranscriptReadDone {
		t.Fatalf("archived source aborted reading: %s", read.Status)
	}
	if len(read.ProposalIDs) != 1 {
		t.Fatalf("archived meeting lost its cited next step: %v", read.ProposalIDs)
	}
	if _, err := e.Activities.GetActivity(e.ctx, meetingID, storekit.LiveOnly); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("reading a transcript exposed its archived meeting: %v", err)
	}
}
