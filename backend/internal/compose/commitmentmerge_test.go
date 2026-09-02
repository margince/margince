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
	}, 0)

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
		0)

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

// An unbounded ask keeps every promise. Zero means "no limit" on this seam, and
// silently trimming would drop work nobody asked to hide.
func TestAnUnboundedAskKeepsEveryPromise(t *testing.T) {
	got, truncated := rankPromises(swept,
		[]agents.OpenCommitment{filedTask("a", -3)},
		[]agents.OpenCommitment{saidInConversation("b", -1)},
		0)

	if len(got) != 2 {
		t.Errorf("returned %d promises with no limit set, want both", len(got))
	}
	if truncated {
		t.Error("reported truncation without cutting anything")
	}
}
