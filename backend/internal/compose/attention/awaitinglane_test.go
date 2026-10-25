// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAFollowUpRowSaysWhoWroteLastAndHowLongAgo(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	sent := ids.NewV7()
	row := classifyAwaiting(AwaitedReply{
		ActivityID: sent, Subject: "the proposal we discussed", SentAt: at.Add(-3 * 24 * time.Hour),
		ContactID: ids.NewV7(), EmailSummary: &crmcontracts.EmailSummary{},
	}, at, time.UTC).item

	if row.SourceRecord == nil || row.SourceRecord.Type != "activity" || ids.UUID(row.SourceRecord.Id) != sent {
		t.Fatalf("message source = %+v", row.SourceRecord)
	}
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

// The composer drafts from the message. A row whose message the reader may not
// read offers no reply. Neither does a row that names no record to file under.
func TestAFollowUpOffersReplyOnlyWhenTheComposerCanDraftIt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	for name, awaited := range map[string]AwaitedReply{
		"no summary": {ActivityID: ids.NewV7(), SentAt: at, ContactID: ids.NewV7()},
		"no record":  {ActivityID: ids.NewV7(), SentAt: at, EmailSummary: &crmcontracts.EmailSummary{}},
	} {
		row := classifyAwaiting(awaited, at, time.UTC).item
		if slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
			t.Errorf("%s: actions = %v, want no reply", name, row.Actions)
		}
	}
}

// The rows are the reader's own sends. A manager reading a colleague's queue
// or the unassigned one must not see them there. The team and all views
// include the reader's own day and keep them.
func TestFollowUpsAreReadOnlyWhereTheReadersOwnDayIs(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	reader := &countingAwaiting{rows: []AwaitedReply{{ActivityID: ids.NewV7(), SentAt: at}}}
	for _, scope := range []TaskScope{TasksOwnedBy, TasksUnassigned} {
		s := &Service{awaiting: reader, taskScope: scope}
		scoped, refusals := s.readingAwaiting(context.Background(), at, time.UTC)
		if len(refusals) != 0 || len(scoped.followUps.rows) != 0 || scoped.followUps.read {
			t.Errorf("scope %v read %d follow-ups, want none", scope, len(scoped.followUps.rows))
		}
	}
	if reader.calls != 0 {
		t.Fatalf("the reader was asked %d times outside the reader's own day", reader.calls)
	}
	for _, scope := range []TaskScope{TasksMine, TasksVisible} {
		s := &Service{awaiting: reader, taskScope: scope, now: func() time.Time { return at }}
		scoped, refusals := s.readingAwaiting(context.Background(), at, time.UTC)
		if refusals[0] != nil || refusals[1] != nil || len(scoped.followUps.rows) != 1 {
			t.Errorf("scope %v read %d follow-ups, want the reader's one", scope, len(scoped.followUps.rows))
		}
	}
}

type countingAwaiting struct {
	rows        []AwaitedReply
	meetings    []AwaitedReply
	calls       int
	meetingsErr error
}

func (c *countingAwaiting) AwaitingReplies(context.Context, time.Time) ([]AwaitedReply, bool, error) {
	c.calls++
	return c.rows, false, nil
}

func (c *countingAwaiting) MeetingFollowUps(context.Context, time.Time) ([]AwaitedReply, bool, error) {
	return c.meetings, false, c.meetingsErr
}

// The two reads fail apart: a refused or failed meeting read names its own
// source and keeps the replies that were read.
func TestAFailedMeetingReadKeepsTheReplies(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	for err, want := range map[error]crmcontracts.WorklistSourceUnavailableReason{
		apperrors.ErrPermissionDenied:        crmcontracts.WorklistSourceUnavailableReasonWithheld,
		errors.New("the database went away"): crmcontracts.WorklistSourceUnavailableReasonFailed,
	} {
		reader := &countingAwaiting{rows: []AwaitedReply{{ActivityID: ids.NewV7(), SentAt: at}}, meetingsErr: err}
		s := &Service{awaiting: reader, taskScope: TasksMine, now: func() time.Time { return at }}
		scoped, refusals := s.readingAwaiting(context.Background(), at, time.UTC)
		if len(scoped.followUps.rows) != 1 || refusals[0] != nil {
			t.Errorf("%v: kept %d replies, refusal %v; want the one reply kept", err, len(scoped.followUps.rows), refusals[0])
		}
		if refusals[1] == nil || refusals[1].Source != sourceMeetingFollowUp || refusals[1].Reason != want {
			t.Errorf("%v: meeting refusal = %+v, want %s on %s", err, refusals[1], want, sourceMeetingFollowUp)
		}
	}
}

// A meeting is not a message to answer. The row offers a fresh message to
// everyone it was with, and names the first of them as its contact.
func TestAMeetingFollowUpOffersAMessageToEveryoneItWasWith(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	first, second, meeting := ids.NewV7(), ids.NewV7(), ids.NewV7()
	ranked, actionable := classifyMeetingFollowUp(AwaitedReply{
		ActivityID: meeting, Subject: "Discovery workshop", SentAt: at.Add(-3 * 24 * time.Hour),
		ContactID: first, DealID: ids.NewV7(), Recipients: []ids.UUID{first, second},
	}, at, time.UTC)
	row := ranked.item
	if row.SourceRecord == nil || row.SourceRecord.Type != "activity" || ids.UUID(row.SourceRecord.Id) != meeting {
		t.Fatalf("meeting source = %+v", row.SourceRecord)
	}
	if !actionable || row.Source != sourceMeetingFollowUp || row.Category != crmcontracts.WorklistItemCategoryMeetings {
		t.Fatalf("row filed as %s/%s (produced: %v)", row.Source, row.Category, actionable)
	}
	if row.Move != nil {
		t.Fatalf("move = %+v, want none: there is no message to draft a reply to", row.Move)
	}
	if !slices.Contains(row.Actions, crmcontracts.WorklistItemActionsReply) {
		t.Fatalf("actions = %v, want reply to the attendees", row.Actions)
	}
	if row.Recipients == nil || len(*row.Recipients) != 2 || ids.UUID((*row.Recipients)[1].Id) != second {
		t.Fatalf("recipients = %+v, want both attendees", row.Recipients)
	}
	if row.Contact == nil || ids.UUID(row.Contact.Id) != first {
		t.Errorf("contact = %+v, want the first recipient", row.Contact)
	}
}

// With nobody on file the row still has a way forward. It opens the record
// the meeting is filed under, and says why there is no draft.
func TestAMeetingWithNoAttendeeOnFileOpensItsRecord(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	for name, met := range map[string]AwaitedReply{
		"deal":    {ActivityID: ids.NewV7(), SentAt: at, DealID: ids.NewV7(), CompanyID: ids.NewV7()},
		"company": {ActivityID: ids.NewV7(), SentAt: at, CompanyID: ids.NewV7()},
	} {
		ranked, actionable := classifyMeetingFollowUp(met, at, time.UTC)
		row := ranked.item
		if !actionable || row.Subject == nil || string(row.Subject.Type) != name {
			t.Fatalf("%s: produced %v with subject %+v, want the %s opened", name, actionable, row.Subject, name)
		}
		if !slices.Equal(row.Actions, []crmcontracts.WorklistItemActions{crmcontracts.WorklistItemActions(actionOpen)}) || row.Recipients != nil {
			t.Errorf("%s: actions = %v, recipients = %v; want open alone", name, row.Actions, row.Recipients)
		}
		if !slices.ContainsFunc(row.Because, func(r crmcontracts.WorklistReason) bool {
			return r.Kind == crmcontracts.WorklistReasonKindNoAttendeeOnFile
		}) {
			t.Errorf("%s: reasons = %+v, want one saying no attendee is on file", name, row.Because)
		}
	}
	if _, actionable := classifyMeetingFollowUp(AwaitedReply{ActivityID: ids.NewV7(), SentAt: at}, at, time.UTC); actionable {
		t.Error("a meeting with no attendee, deal or company on file produced a row")
	}
}

// No follow-up row reaches the queue without a verb, whatever the read named.
func TestNoMeetingFollowUpRowIsDrawnWithoutAnAction(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	someone := ids.NewV7()
	reads := []AwaitedReply{}
	for _, contact := range [][]ids.UUID{nil, {someone}} {
		for _, deal := range []ids.UUID{{}, ids.NewV7()} {
			for _, company := range []ids.UUID{{}, ids.NewV7()} {
				met := AwaitedReply{ActivityID: ids.NewV7(), SentAt: at, DealID: deal, CompanyID: company, Recipients: contact}
				if len(contact) > 0 {
					met.ContactID = contact[0]
				}
				reads = append(reads, met)
			}
		}
	}
	s := &Service{awaiting: &countingAwaiting{meetings: reads}, taskScope: TasksMine, now: func() time.Time { return at }}
	scoped, _ := s.readingAwaiting(context.Background(), at, time.UTC)
	if len(scoped.followUps.rows) != len(reads)-1 {
		t.Fatalf("%d rows from %d reads, want every read but the one naming no record", len(scoped.followUps.rows), len(reads))
	}
	for _, row := range scoped.followUps.rows {
		if len(row.item.Actions) == 0 {
			t.Errorf("row %s was drawn with no action: %+v", row.item.Id, row.item)
		}
	}
}

func TestAFollowUpToADayEmailSaysItsTimeIsADate(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 9, 6, 9, 0, 0, 0, time.UTC)
	awaited := AwaitedReply{
		ActivityID: ids.NewV7(), SentAt: at.Add(-3 * 24 * time.Hour), ContactID: ids.NewV7(),
		Precision: crmcontracts.OccurredPrecisionOccurredPrecisionDay,
	}
	if got := classifyAwaiting(awaited, at, time.UTC).item.OccurredPrecision; got != crmcontracts.OccurredPrecisionOccurredPrecisionDay {
		t.Errorf("precision = %q, want day", got)
	}
	awaited.Precision = ""
	if got := classifyAwaiting(awaited, at, time.UTC).item.OccurredPrecision; got != crmcontracts.OccurredPrecisionOccurredPrecisionInstant {
		t.Errorf("precision = %q, want instant", got)
	}
}

func metDaysOf(t *testing.T, row crmcontracts.WorklistItem) int {
	t.Helper()
	for _, because := range row.Because {
		if because.Kind == "met_days_ago" && because.Value != nil && because.Value.Days != nil {
			return *because.Value.Days
		}
	}
	t.Fatalf("no met_days_ago reason with a day count in %+v", row.Because)
	return 0
}

// The count follows the installation's calendar. A meeting at 22:00 and a read
// at 01:00 the next morning are one day apart by the wall, three hours by the clock.
func TestAMeetingFollowUpCountsCalendarDaysInTheInstallationZone(t *testing.T) {
	t.Parallel()
	hcm, err := time.LoadLocation("Asia/Ho_Chi_Minh")
	if err != nil {
		t.Fatalf("load zone: %v", err)
	}
	met := time.Date(2026, 9, 6, 22, 0, 0, 0, hcm)
	for name, c := range map[string]struct {
		asOf time.Time
		want int
	}{
		"same local day":         {time.Date(2026, 9, 6, 23, 30, 0, 0, hcm), 0},
		"after local midnight":   {time.Date(2026, 9, 7, 1, 0, 0, 0, hcm), 1},
		"a day and a half later": {time.Date(2026, 9, 8, 10, 0, 0, 0, hcm), 2},
	} {
		ranked, actionable := classifyMeetingFollowUp(
			AwaitedReply{ActivityID: ids.NewV7(), SentAt: met, DealID: ids.NewV7()}, c.asOf, hcm)
		if !actionable {
			t.Fatalf("%s: no row", name)
		}
		if got := metDaysOf(t, ranked.item); got != c.want || ranked.waitingDays != c.want {
			t.Errorf("%s: row says %d, ranks on %d, want %d", name, got, ranked.waitingDays, c.want)
		}
	}
}
