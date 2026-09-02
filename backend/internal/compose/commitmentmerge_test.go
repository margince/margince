// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

var swept = time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)

func promiseDue(days int) *time.Time {
	at := swept.AddDate(0, 0, days)
	return &at
}

func filedTask(subject string, dueInDays int) agents.OpenCommitment {
	id := ids.NewV7()
	return agents.OpenCommitment{
		Source: agents.CommitmentFromTask, TaskID: &id,
		Subject: subject, DueAt: promiseDue(dueInDays),
	}
}

func saidInConversation(subject string, dueInDays int) agents.OpenCommitment {
	claim, source := ids.NewV7(), ids.NewV7()
	return agents.OpenCommitment{
		Source: agents.CommitmentFromConversation, ClaimID: &claim, SourceActivityID: &source,
		Quote: "Ich schicke es Ihnen.", Subject: subject, DueAt: promiseDue(dueInDays),
	}
}

// The tool answered from tasks alone, so a promise made in a meeting and never
// typed was reported as absent — and a model told "these are the open
// commitments" repeats that as fact.
func TestAPromiseMadeInAConversationReachesTheAnswer(t *testing.T) {
	got, _ := rankPromises(swept, nil, []agents.OpenCommitment{
		saidInConversation("Send the security questionnaire", -2),
	}, agents.CommitmentSweepLimit(0))

	if len(got) != 1 {
		t.Fatalf("answered with %d promises; a commitment nobody typed is still owed", len(got))
	}
	if got[0].Source != agents.CommitmentFromConversation {
		t.Errorf("source = %q, want conversation", got[0].Source)
	}
	if got[0].Quote == "" {
		t.Error("no quote; the sentence the promise was made in is what a claim carries")
	}
	if got[0].TaskID != nil {
		t.Error("a conversation promise carries no task id — nobody filed one")
	}
}

// Both sources rank by one rule, so an agent and a reader looking at the same
// account agree about what is most overdue.
func TestBothSourcesRankByOneRule(t *testing.T) {
	got, _ := rankPromises(swept,
		[]agents.OpenCommitment{filedTask("Return the redlines", -20)},
		[]agents.OpenCommitment{saidInConversation("Send the quote", -1)},
		agents.CommitmentSweepLimit(0))

	if len(got) != 2 {
		t.Fatalf("merged into %d promises, want both", len(got))
	}
	if got[0].Subject != "Send the quote" {
		t.Errorf("led with %q; the promise that slipped most recently comes first", got[0].Subject)
	}
}

// The caller asked for a bounded set and gets exactly that, plus the fact that
// more exist. A merged answer can exceed the bound each source respected.
func TestTheMergedAnswerHonoursTheCallersBound(t *testing.T) {
	got, truncated := rankPromises(swept,
		[]agents.OpenCommitment{filedTask("a", -3), filedTask("b", -2)},
		[]agents.OpenCommitment{saidInConversation("c", -1)},
		2)

	if len(got) != 2 {
		t.Errorf("returned %d promises for a limit of 2", len(got))
	}
	if !truncated {
		t.Error("cut a promise from the answer and did not say so; a model reports a " +
			"bounded set as everything outstanding unless told otherwise")
	}
}

// An answer that fits under the bound keeps every promise and says nothing was
// cut. Reporting truncation on a complete answer would have a model hedge a
// fact it could have stated.
func TestAnAnswerThatFitsReportsNoTruncation(t *testing.T) {
	got, truncated := rankPromises(swept,
		[]agents.OpenCommitment{filedTask("a", -3)},
		[]agents.OpenCommitment{saidInConversation("b", -1)},
		agents.CommitmentSweepLimit(0))

	if len(got) != 2 {
		t.Errorf("returned %d promises, want both", len(got))
	}
	if truncated {
		t.Error("reported truncation without cutting anything")
	}
}

// The bound belongs to the tool, not to either read behind it. The two sources
// default differently — fifty for tasks, two hundred for claims — so a caller
// who omits the limit would get whatever the two happened to add up to, and be
// told nothing was cut.
func TestAnOmittedLimitIsTheToolsOwnCeiling(t *testing.T) {
	var said []agents.OpenCommitment
	for i := range 60 {
		said = append(said, saidInConversation("promise", -i-1))
	}

	got, truncated := rankPromises(swept, nil, said, agents.CommitmentSweepLimit(0))

	// Both ends, because either alone passes for the wrong reason: a ceiling
	// with no floor is satisfied by returning nothing, and a floor with no
	// ceiling by returning everything both reads happened to fetch.
	if len(got) != 50 {
		t.Errorf("returned %d promises for a caller who named no limit; the tool serves exactly "+
			"its ceiling of 50 when more exist", len(got))
	}
	if !truncated {
		t.Error("cut ten promises and reported nothing; a model reads a bounded set as everything outstanding")
	}
}

// Two promises due the same day rank by when they were promised, whichever
// source holds them. Left to input order, every task would beat every claim on
// a tie regardless of which was actually made first.
func TestASharedDeadlineBreaksOnWhenThePromiseWasMade(t *testing.T) {
	task := filedTask("Return the redlines", 8)
	task.FiledAt = swept.AddDate(0, 0, -1)
	claim := saidInConversation("Send the quote", 8)
	claim.FiledAt = swept.AddDate(0, 0, -30)

	got, _ := rankPromises(swept, []agents.OpenCommitment{task}, []agents.OpenCommitment{claim}, 10)

	if got[0].Subject != "Send the quote" {
		t.Errorf("led with %q; of two promises due the same day the one promised first leads, "+
			"and the claim was made a month before the task", got[0].Subject)
	}
}
