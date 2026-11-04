// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A transcript reading's outcome, from the facts the run stores to the
// sentence each reader is shown. The run keeps who each task went to and who
// each proposal waits on; the report says "you" for the reader's own and a
// name for anyone else's, in the reader's language.

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// transcriptStaging is what one reading did with the promises it found.
type transcriptStaging struct {
	proposals []activities.TranscriptProposal
	tasks     map[ids.UUID]int
	known     map[ids.UUID]int
	watched   int
	withheld  map[CommitmentOutcome]int
}

func newTranscriptStaging() transcriptStaging {
	return transcriptStaging{tasks: map[ids.UUID]int{}, known: map[ids.UUID]int{}, withheld: map[CommitmentOutcome]int{}}
}

// approvalIDs are the staged approvals in the order they were staged.
func (s transcriptStaging) approvalIDs() []ids.UUID {
	out := make([]ids.UUID, 0, len(s.proposals))
	for _, proposal := range s.proposals {
		out = append(out, proposal.ApprovalID)
	}
	return out
}

// facts is the staging as the run record keeps it. found is how many promises
// the reading kept before the rule sorted them.
func (s transcriptStaging) facts(found int) *activities.TranscriptReadFacts {
	out := &activities.TranscriptReadFacts{Found: found, Watched: s.watched, Proposals: s.proposals}
	out.Tasks = seatCounts(s.tasks)
	out.Known = seatCounts(s.known)
	for _, reason := range slices.Sorted(maps.Keys(s.withheld)) {
		out.Withheld = append(out.Withheld, activities.TranscriptReasonCount{Reason: string(reason), Count: s.withheld[reason]})
	}
	return out
}

func compareSeats(a, b ids.UUID) int { return cmp.Compare(a.String(), b.String()) }

func seatCounts(bySeat map[ids.UUID]int) []activities.TranscriptSeatCount {
	var out []activities.TranscriptSeatCount
	for _, seat := range slices.SortedFunc(maps.Keys(bySeat), compareSeats) {
		out = append(out, activities.TranscriptSeatCount{SeatID: seat, Count: bySeat[seat]})
	}
	return out
}

// transcriptTaskAssignee is the seat a task the rule wrote is assigned to.
func transcriptTaskAssignee(ctx context.Context, tx pgx.Tx, task ids.UUID) (ids.UUID, error) {
	var seat *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT assignee_id FROM activity WHERE id = $1`, task).Scan(&seat); err != nil {
		return ids.UUID{}, fmt.Errorf("compose: reading who a transcript task went to: %w", err)
	}
	if seat == nil {
		return ids.UUID{}, nil
	}
	return *seat, nil
}

// proposalDecider is the seat a staged proposal waits on.
func proposalDecider(ctx context.Context, tx pgx.Tx, approval ids.UUID) (ids.UUID, error) {
	var seat *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT COALESCE(requested_decider_id, on_behalf_of) FROM approval WHERE id = $1`,
		approval).Scan(&seat); err != nil {
		return ids.UUID{}, fmt.Errorf("compose: reading who a transcript proposal waits on: %w", err)
	}
	if seat == nil {
		return ids.UUID{}, nil
	}
	return *seat, nil
}

// transcriptAudience is who a sentence about a reading is for. A zero reader
// is nobody in particular: the stored sentence names no one.
type transcriptAudience struct {
	reader ids.UUID
	names  map[ids.UUID]string
}

func (a transcriptAudience) personal() bool { return !a.reader.IsZero() }

// outcomeSentence phrases what a reading did.
func (said transcriptCopy) outcomeSentence(facts activities.TranscriptReadFacts, who transcriptAudience) string {
	if facts.Failure != nil {
		return said.failureSentence(*facts.Failure)
	}
	details := said.taskPhrases(facts.Tasks, who)
	if facts.Watched > 0 {
		details = append(details, said.count(facts.Watched, transcriptWords.WatchedOne, transcriptWords.Watched))
	}
	details = append(details, said.proposalPhrases(facts.Proposals, who)...)
	details = append(details, said.knownPhrases(facts.Known, who)...)
	for _, withheld := range facts.Withheld {
		details = append(details, said.count(withheld.Count, transcriptWords.WithheldOne, transcriptWords.Withheld,
			said.transcriptWithheldReason(CommitmentOutcome(withheld.Reason))))
	}
	if len(details) > 0 {
		return strings.Join(details, "; ")
	}
	if facts.Found == 0 {
		return said.Say(transcriptWords.Empty)
	}
	return said.Say(transcriptWords.AlreadyKnown)
}

// taskPhrases says who the written tasks went to, the reader's own first.
func (said transcriptCopy) taskPhrases(tasks []activities.TranscriptSeatCount, who transcriptAudience) []string {
	if !who.personal() {
		total := 0
		for _, task := range tasks {
			total += task.Count
		}
		if total == 0 {
			return nil
		}
		return []string{said.count(total, transcriptWords.TasksOne, transcriptWords.Tasks)}
	}
	var mine, theirs []string
	for _, task := range tasks {
		if task.SeatID == who.reader {
			mine = append(mine, said.count(task.Count, transcriptWords.TasksYouOne, transcriptWords.TasksYou))
			continue
		}
		theirs = append(theirs, said.count(task.Count, transcriptWords.TasksForOne, transcriptWords.TasksFor, said.seatName(task.SeatID, who)))
	}
	return append(mine, theirs...)
}

// knownPhrases says who the promises already put to someone were put to. A
// promise nobody could be named for is only "already known".
func (said transcriptCopy) knownPhrases(known []activities.TranscriptSeatCount, who transcriptAudience) []string {
	var mine, theirs []string
	nobody := 0
	for _, entry := range known {
		switch {
		case entry.SeatID.IsZero() || !who.personal():
			nobody += entry.Count
		case entry.SeatID == who.reader:
			mine = append(mine, said.count(entry.Count, transcriptWords.KnownYouOne, transcriptWords.KnownYou))
		default:
			theirs = append(theirs, said.count(entry.Count, transcriptWords.KnownToOne, transcriptWords.KnownTo, said.seatName(entry.SeatID, who)))
		}
	}
	out := append(mine, theirs...)
	if nobody > 0 {
		out = append(out, said.count(nobody, transcriptWords.KnownOne, transcriptWords.Known))
	}
	return out
}

// proposalPhrases says which proposals wait on the reader and which were sent
// to a colleague to confirm.
func (said transcriptCopy) proposalPhrases(proposals []activities.TranscriptProposal, who transcriptAudience) []string {
	if len(proposals) == 0 {
		return nil
	}
	if !who.personal() {
		return []string{said.count(len(proposals), transcriptWords.ProposalsWaitOne, transcriptWords.ProposalsWait)}
	}
	mine := 0
	others := map[ids.UUID]int{}
	for _, proposal := range proposals {
		if proposal.SeatID == who.reader {
			mine++
		} else {
			others[proposal.SeatID]++
		}
	}
	var out []string
	if mine > 0 {
		out = append(out, said.count(mine, transcriptWords.ProposalsOne, transcriptWords.Proposals))
	}
	for _, seat := range slices.SortedFunc(maps.Keys(others), compareSeats) {
		out = append(out, said.count(others[seat], transcriptWords.SentToOne, transcriptWords.SentTo, said.seatName(seat, who)))
	}
	return out
}

// seatName is the seat as the reader knows it, or "a colleague" for a seat
// with no name to show.
func (said transcriptCopy) seatName(seat ids.UUID, who transcriptAudience) string {
	if name := strings.TrimSpace(who.names[seat]); name != "" {
		return name
	}
	return said.Say(transcriptWords.AColleague)
}

// failureSentence phrases a reading that could not be used.
func (said transcriptCopy) failureSentence(failure activities.TranscriptFailure) string {
	switch failure.Code {
	case activities.TranscriptFailureNotTranscript:
		return said.Say(transcriptWords.NotTranscript)
	case activities.TranscriptFailureTooLong:
		return fmt.Sprintf(said.Say(transcriptWords.TooLong), failure.Lines, failure.Chars,
			activities.MaxReadableTranscriptLines, activities.MaxReadableTranscriptChars)
	case activities.TranscriptFailureBlank:
		return said.Say(transcriptWords.Blank)
	case activities.TranscriptFailureMissing:
		return said.Say(transcriptWords.Missing)
	case activities.TranscriptFailureNoModel:
		return said.Say(transcriptWords.NoModel)
	default:
		return said.Say(transcriptWords.Refused)
	}
}

// unreadableTranscript separates a refusal a rep can act on from a fault the
// job should retry. Only the typed refusals become a failure code: a raw
// driver error would settle a transient blip as a permanent verdict.
func unreadableTranscript(err error) (*activities.TranscriptFailure, bool) {
	var notTranscript *activities.NotATranscriptError
	var tooLong *activities.TranscriptTooLongError
	switch {
	case errors.As(err, &notTranscript):
		return &activities.TranscriptFailure{Code: activities.TranscriptFailureNotTranscript}, true
	case errors.As(err, &tooLong):
		return &activities.TranscriptFailure{Code: activities.TranscriptFailureTooLong, Lines: tooLong.Lines, Chars: tooLong.Chars}, true
	case errors.Is(err, activities.ErrBlankTranscript):
		return &activities.TranscriptFailure{Code: activities.TranscriptFailureBlank}, true
	case errors.Is(err, apperrors.ErrNotFound):
		return &activities.TranscriptFailure{Code: activities.TranscriptFailureMissing}, true
	}
	return nil, false
}

// failedOutcome closes a reading that could not be used. The stored sentence
// is the base language's, for readers of the record that predate facts.
func (said transcriptCopy) failedOutcome(failure activities.TranscriptFailure) activities.TranscriptReadOutcome {
	return activities.TranscriptReadOutcome{
		Status: activities.TranscriptReadFailed,
		Detail: said.failureSentence(failure),
		Facts:  &activities.TranscriptReadFacts{Failure: &failure},
	}
}
