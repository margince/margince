// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// A decline stands against the prompt that was declined, and against no other.
//
// The backlog, the trace's explanation and the stamping write each read the
// decline through one predicate. These pin that the three agree over real rows
// when the prompt moves: a row declined under older wording is offered again,
// explained as waiting rather than declined, and re-stamped once under the new
// wording rather than on every tick.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
)

func TestADeclineUnderAnotherPromptIsOfferedToThisOne(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()
	declined := e.seed(t, capturedRow{kind: "email"})
	if recorded, err := e.store.MarkCaptureLabelDeclined(ctx, declined, rulesetOld); err != nil || !recorded {
		t.Fatalf("recording the decline under the old prompt: recorded=%v err=%v", recorded, err)
	}

	for _, tc := range []struct {
		ruleset      string
		wantOffered  bool
		wantReason   pipelinetrace.Reason
		whatItProves string
	}{
		{rulesetOld, false, pipelinetrace.ReasonModelsDeclined, "the prompt that was declined does not ask again"},
		{rulesetNew, true, pipelinetrace.ReasonAwaitingBatch, "a reworded prompt is offered what the old one was refused"},
	} {
		backlog, err := e.store.UnlabeledCaptureEmails(ctx, tc.ruleset, 50, 200)
		if err != nil {
			t.Fatalf("reading the backlog under %s: %v", tc.ruleset, err)
		}
		if offered := backlogHolds(backlog, declined); offered != tc.wantOffered {
			t.Errorf("under %s the backlog offered the row: %v, want %v — %s", tc.ruleset, offered, tc.wantOffered, tc.whatItProves)
		}
		facts, err := e.store.ReadPipelineFacts(ctx, declined, tc.ruleset)
		if err != nil {
			t.Fatalf("reading pipeline facts under %s: %v", tc.ruleset, err)
		}
		if facts.ClassifyEligible != tc.wantOffered || facts.ClassifyReason != tc.wantReason {
			t.Errorf("under %s the trace says eligible=%v reason=%q, want eligible=%v reason=%q — the explanation "+
				"and the backlog disagree", tc.ruleset, facts.ClassifyEligible, facts.ClassifyReason, tc.wantOffered, tc.wantReason)
		}
	}
}

func TestADeclineIsRestampedOncePerPrompt(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()
	id := e.seed(t, capturedRow{kind: "email"})
	for _, step := range []struct {
		ruleset string
		want    bool
		why     string
	}{
		{rulesetOld, true, "the first decline is recorded"},
		{rulesetOld, false, "a second pass under the same prompt leaves the first stamp standing"},
		{rulesetNew, true, "a decline under a new prompt replaces the stale stamp"},
		{rulesetNew, false, "and then stands for that prompt"},
	} {
		recorded, err := e.store.MarkCaptureLabelDeclined(ctx, id, step.ruleset)
		if err != nil {
			t.Fatalf("stamping under %s: %v", step.ruleset, err)
		}
		if recorded != step.want {
			t.Errorf("stamping under %s recorded=%v, want %v: %s", step.ruleset, recorded, step.want, step.why)
		}
	}
	backlog, err := e.store.UnlabeledCaptureEmails(ctx, rulesetNew, 50, 200)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	if backlogHolds(backlog, id) {
		t.Error("a row declined under the current prompt is still offered, so the sweep would re-send it every tick")
	}
}

// A decline recorded before the digest existed names no prompt, and reads as
// declined under another one: it is offered once more under the current prompt,
// and the write that records a second refusal re-stamps it. No writer produces
// that row any more, so the seed writes the stamp the old writer left.
func TestALegacyDeclineIsOfferedOnceAndRestamped(t *testing.T) {
	e := setupFacts(t)
	ctx := e.as()
	id := e.seed(t, capturedRow{kind: "email"})
	e.exec(t, `UPDATE activity SET capture_label_declined_at = now() WHERE id = $1`, id)

	backlog, err := e.store.UnlabeledCaptureEmails(ctx, rulesetNew, 50, 200)
	if err != nil {
		t.Fatalf("reading the backlog: %v", err)
	}
	if !backlogHolds(backlog, id) {
		t.Error("a decline naming no prompt was not offered to the current one")
	}
	if recorded, err := e.store.MarkCaptureLabelDeclined(ctx, id, rulesetNew); err != nil || !recorded {
		t.Errorf("re-stamping the legacy decline: recorded=%v err=%v, want it recorded under the current prompt", recorded, err)
	}
}

func backlogHolds(backlog []UnlabeledEmail, id ids.UUID) bool {
	for _, m := range backlog {
		if m.ID == id {
			return true
		}
	}
	return false
}
