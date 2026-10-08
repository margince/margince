// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAFollowUpRowSaysWhoWroteLastAndHowLongAgo(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	sent := ids.NewV7()
	row := classifyAwaiting(AwaitedReply{
		ActivityID: sent, Subject: "the proposal we discussed", SentAt: at.Add(-3 * 24 * time.Hour),
		ContactID: ids.NewV7(), EmailSummary: &crmcontracts.EmailSummary{},
	}, at).item

	if row.Source != sourceAwaitingReply || row.Category != "tasks" {
		t.Fatalf("row filed as %s/%s, want %s/tasks", row.Source, row.Category, sourceAwaitingReply)
	}
	if len(row.Because) != 2 || row.Because[0].Kind != "you_wrote_last" || row.Because[1].Kind != "no_reply_days" {
		t.Fatalf("because = %+v, want you_wrote_last then no_reply_days", row.Because)
	}
	if row.Move == nil || row.Move.Action != crmcontracts.WorklistMoveActionDraftReply ||
		row.Move.ActivityId == nil || ids.UUID(*row.Move.ActivityId) != sent {
		t.Fatalf("move = %+v, want a follow-up drafted from the sent message", row.Move)
	}
	if !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Fatalf("actions = %v, want reply on a message the reader may read", row.Actions)
	}
}

// The composer drafts from the message, so a row whose message the reader may
// not read, or that names no record to file the follow-up against, offers no
// reply.
func TestAFollowUpOffersReplyOnlyWhenTheComposerCanDraftIt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	for name, awaited := range map[string]AwaitedReply{
		"no summary": {ActivityID: ids.NewV7(), SentAt: at, ContactID: ids.NewV7()},
		"no record":  {ActivityID: ids.NewV7(), SentAt: at, EmailSummary: &crmcontracts.EmailSummary{}},
	} {
		row := classifyAwaiting(awaited, at).item
		if slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
			t.Errorf("%s: actions = %v, want no reply", name, row.Actions)
		}
	}
}

// The rows are the reader's own sends. A manager reading a colleague's queue
// or the unassigned one must not see their own follow-ups there; the team and
// all views include the reader's own day and keep them.
func TestFollowUpsAreReadOnlyWhereTheReadersOwnDayIs(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	reader := &countingAwaiting{rows: []AwaitedReply{{ActivityID: ids.NewV7(), SentAt: at}}}
	for _, scope := range []TaskScope{TasksOwnedBy, TasksUnassigned} {
		s := &Service{awaiting: reader, taskScope: scope}
		scoped, refusal := s.readingAwaiting(context.Background(), at)
		if refusal != nil || len(scoped.followUps.rows) != 0 || scoped.followUps.read {
			t.Errorf("scope %v read %d follow-ups, want none", scope, len(scoped.followUps.rows))
		}
	}
	if reader.calls != 0 {
		t.Fatalf("the reader was asked %d times outside the reader's own day", reader.calls)
	}
	for _, scope := range []TaskScope{TasksMine, TasksVisible} {
		s := &Service{awaiting: reader, taskScope: scope, now: func() time.Time { return at }}
		scoped, refusal := s.readingAwaiting(context.Background(), at)
		if refusal != nil || len(scoped.followUps.rows) != 1 {
			t.Errorf("scope %v read %d follow-ups, want the reader's one", scope, len(scoped.followUps.rows))
		}
	}
}

type countingAwaiting struct {
	rows  []AwaitedReply
	calls int
}

func (c *countingAwaiting) AwaitingReplies(context.Context, time.Time) ([]AwaitedReply, bool, error) {
	c.calls++
	return c.rows, false, nil
}

func (c *countingAwaiting) MeetingFollowUps(context.Context, time.Time) ([]AwaitedReply, bool, error) {
	return nil, false, nil
}

// A meeting is not a message to answer: the row offers a fresh message to the
// contact, and only when it names one.
func TestAMeetingFollowUpOffersAMessageToTheContactItWasWith(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	row := classifyMeetingFollowUp(AwaitedReply{
		ActivityID: ids.NewV7(), Subject: "Discovery workshop", SentAt: at.Add(-3 * 24 * time.Hour),
		ContactID: ids.NewV7(),
	}, at).item
	if row.Source != sourceMeetingFollowUp || row.Category != crmcontracts.WorklistItemCategoryMeetings {
		t.Fatalf("row filed as %s/%s", row.Source, row.Category)
	}
	if row.Move != nil {
		t.Fatalf("move = %+v, want none: there is no message to draft a reply to", row.Move)
	}
	if !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Fatalf("actions = %v, want reply to the contact", row.Actions)
	}
	nobody := classifyMeetingFollowUp(AwaitedReply{ActivityID: ids.NewV7(), SentAt: at, CompanyID: ids.NewV7()}, at).item
	if slices.Contains(nobody.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Fatalf("a meeting naming no contact offers a message: %v", nobody.Actions)
	}
}
