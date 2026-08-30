// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// What a rep's "no" means when the same transcript is read again.
//
// Reading a transcript twice is an ordinary thing to do: a rep re-checks a call,
// or a colleague opens it. The in-flight uniqueness index only stops a SECOND
// reading while the first is queued or running, so once a reading finishes
// another may start — and the model, asked the same question of the same text,
// says roughly the same thing. Without a memory the refused proposals come
// straight back, and the rep learns that turning something down does not stick.
//
// "Roughly" is what makes the key hard. The summary is the model's prose and the
// cited line is its citation; both move between readings of one document. The
// transcript's own words do not, which is what the identity carries.

import (
	"encoding/json"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reworded is a reading that reaches the same commitment on the same line and
// says so differently — what a second pass over one transcript actually looks
// like.
func reworded(t *testing.T, summary string, line int) string {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"proposals": []map[string]any{{
		"summary": summary, "owner": "Priya",
		"source_lines": []int{line}, "confidence": 0.9,
	}}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	return string(raw)
}

// pendingProposals counts the transcript proposals waiting on this activity.
func (e *transcriptEnv) pendingProposals(t *testing.T) int {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM approval
		 WHERE kind = 'transcript_proposal' AND status = 'pending'`)
}

// rejectFirst turns down the proposal a reading staged.
func (e *transcriptEnv) rejectFirst(t *testing.T, proposalIDs []ids.UUID) {
	t.Helper()
	reason := "not a commitment"
	if len(proposalIDs) == 0 {
		t.Fatal("the reading staged nothing to reject")
	}
	if _, err := e.svc.Decide(e.ctx, ids.From[ids.ApprovalKind](proposalIDs[0]), false, &reason); err != nil {
		t.Fatalf("rejecting the proposal: %v", err)
	}
}

// A refused proposal does not come back when the transcript is read again.
func TestARejectedTranscriptProposalIsNotStagedAgainOnASecondReading(t *testing.T) {
	e := setupTranscript(t)
	first := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	e.rejectFirst(t, first.ProposalIDs)

	second := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	if got := e.pendingProposals(t); got != 0 {
		t.Errorf("a second reading staged %d proposals over a rejection, want 0 — "+
			"the rep is shown what they already turned down", got)
	}
	if len(second.ProposalIDs) != 0 {
		t.Errorf("the run recorded %d proposals it did not raise", len(second.ProposalIDs))
	}
}

// The same commitment, worded differently, is still the same question.
//
// This is the case the identity exists for. The model is not deterministic, so a
// second reading of one transcript paraphrases: the payload differs, and a
// memory keyed on the payload's diff hash forgets the refusal immediately.
func TestARejectedTranscriptProposalStaysRefusedWhenTheModelRewordsIt(t *testing.T) {
	e := setupTranscript(t)
	first := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	e.rejectFirst(t, first.ProposalIDs)

	// Same line of the transcript, different words for what it says.
	e.read(t, cannedBrain{reply: reworded(t, "Priya to send revised pricing by Friday", 3)})
	if got := e.pendingProposals(t); got != 0 {
		t.Errorf("a reworded reading of the same line staged %d proposals over a "+
			"rejection, want 0 — the memory is keyed on the model's prose", got)
	}
}

// A commitment from a different part of the transcript is a different question.
//
// The other half of the key. A refusal is remembered with no expiry, so a
// target-only key would mean one "no" ends transcript reading on that activity
// for good — every later commitment silently suppressed, with nothing to point
// at.
func TestADifferentLineIsStillProposedAfterARefusal(t *testing.T) {
	e := setupTranscript(t)
	first := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	e.rejectFirst(t, first.ProposalIDs)

	// A different sentence of the same call.
	e.read(t, cannedBrain{reply: reworded(t, "Dana to review the pricing", 4)})
	if got := e.pendingProposals(t); got != 1 {
		t.Errorf("a commitment from a line nobody refused staged %d proposals, want 1 — "+
			"one refusal has ended transcript reading on this activity for good", got)
	}
}

// A second reading does not duplicate a proposal that is still waiting.
//
// The pending half, which the producer had no guard for at all: before this,
// reading twice put two identical cards in front of the rep.
func TestASecondReadingDoesNotDuplicateAProposalStillWaiting(t *testing.T) {
	e := setupTranscript(t)
	e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	if got := e.pendingProposals(t); got != 1 {
		t.Errorf("two readings left %d proposals waiting, want 1 — the rep is shown "+
			"the same commitment twice", got)
	}
}

// A reading that raises nothing says why.
//
// "I read it and everything in it was already put to you" and "I could not read
// it" are different answers, and a run that finishes silent reads as the second.
// The run record refuses to let them collapse, which is what caught this.
func TestAReadingWhoseStepsWereAllAnsweredSaysSo(t *testing.T) {
	e := setupTranscript(t)
	first := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	e.rejectFirst(t, first.ProposalIDs)

	second := e.read(t, cannedBrain{reply: groundedReply(t, 3, 0.9)})
	if second.StatusDetail == nil || *second.StatusDetail == "" {
		t.Fatal("a reading that raised nothing recorded no reason, so it cannot be " +
			"told from one that failed")
	}
	if len(second.ProposalIDs) != 0 {
		t.Errorf("the run claims %d proposals it did not raise", len(second.ProposalIDs))
	}
}

// Two commitments made in one sentence are two proposals, not one.
//
// The identity is the transcript's own words, so two steps citing the SAME line
// carry the same key — and identity staging supersedes a pending twin. If a
// reading can produce that, one commitment silently replaces the other and the
// rep never sees it.
func TestTwoCommitmentsOnOneLineBothReachTheQueue(t *testing.T) {
	e := setupTranscript(t)
	both, err := json.Marshal(map[string]any{"proposals": []map[string]any{
		{
			"summary": "Send the revised pricing", "owner": "Priya",
			"source_lines": []int{3}, "confidence": 0.9,
		},
		{
			"summary": "Book the follow-up call", "owner": "Dana",
			"source_lines": []int{3}, "confidence": 0.9,
		},
	}})
	if err != nil {
		t.Fatalf("building the model reply: %v", err)
	}
	read := e.read(t, cannedBrain{reply: string(both)})

	if got := e.pendingProposals(t); got != 2 {
		t.Errorf("one line stating two commitments left %d proposals waiting, want 2 — "+
			"they share a cited quotation, so one has superseded the other and the rep "+
			"will never see it", got)
	}
	if len(read.ProposalIDs) != 2 {
		t.Errorf("the run recorded %d proposals, want 2", len(read.ProposalIDs))
	}
}
