// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package pipelinetrace

// What each rung is allowed to CLAIM.
//
// Every judgement this feature is about lives in rungs.go, and every one of them
// is a sentence shown to a member. The failures worth catching are not crashes:
// they are a rung that says the stage did not apply when the record was merely
// swept, or that a colleague's message was never captured when the truth is that
// the reader may not be told.
//
// No database: rung() is a pure function of a stored ladder, the derived facts
// and the caller's standing. That is why it can be exhaustive here.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	trace "github.com/margince/margince/backend/internal/shared/kernel/pipelinetrace"
)

func reg(t *testing.T, stage trace.Stage) trace.Registration {
	t.Helper()
	r, ok := trace.Lookup(stage)
	if !ok {
		t.Fatalf("%s is not registered", stage)
	}
	return r
}

func row(stage trace.Stage, outcome, reason string) capture.TraceRow {
	return capture.TraceRow{
		Stage: string(stage), Outcome: outcome, Reason: reason,
		OccurredAt: time.Date(2026, 8, 16, 9, 0, 0, 0, time.UTC),
	}
}

func ladderWith(rows ...capture.TraceRow) capture.TraceLadder {
	return capture.TraceLadder{Rungs: rows}
}

// TestEveryAnsweringStageHasABranch is the gate rungs.go's fallback comment
// claims. Without it, adding a stored or derived stage to the registry with no
// branch here renders `unknown` at a member and nothing fails.
func TestEveryAnsweringStageHasABranch(t *testing.T) {
	// A ladder rich enough that no branch has to fall through for lack of data.
	activityID := ids.NewV7()
	stored := ladderWith(
		row(trace.StageInternalDrop, "internal", "internal_only"),
		row(trace.StageTierLadder, "captured", ""),
	)
	stored.ActivityID = &activityID
	facts := &activities.PipelineFacts{HasContactLink: true, CaptureLabel: "meeting"}

	for _, r := range trace.Registrations() {
		if r.AbsentReason != "" {
			continue // declares its own silence; the switch never sees it
		}
		got := (&Assembler{}).rung(r, view{stored: stored, owned: true}, facts, nil)
		if got.Status == trace.StatusUnknown && got.Reason == "" {
			t.Errorf("%s answers, but the assembler has no branch for it — it fell "+
				"through to the bare unknown, which reads as a real state at a member",
				r.Stage)
		}
	}
}

func TestANonOwnerLearnsNothingFromWhetherARowExists(t *testing.T) {
	// The oracle claim, asserted as EQUALITY of the two answers rather than as
	// the shape of one: a colleague comparing two shared messages must not be
	// able to tell which of them hit a replay fault on somebody else's mailbox.
	withFault := ladderWith(row(trace.StageActivityWrite, "fault", "invisible_incumbent"))
	without := ladderWith()

	for _, stage := range []trace.Stage{
		trace.StageInternalDrop, trace.StageTierLadder, trace.StageVerdict,
	} {
		r := reg(t, stage)
		a := &Assembler{}
		got := a.rung(r, view{stored: withFault}, nil, nil)
		want := a.rung(r, view{stored: without}, nil, nil)
		if got != want {
			t.Errorf("%s differs for a non-owner depending on whether a row exists:\n"+
				" with a row: %+v\n without:    %+v", stage, got, want)
		}
		if got.Status != trace.StatusUnknown {
			t.Errorf("%s status for a non-owner = %q, want unknown", stage, got.Status)
		}
	}
}

func TestASweptWindowDoesNotClaimTheStageDidNotApply(t *testing.T) {
	// The distinction the whole surface turns on. With NO rows, "did not apply"
	// is a claim the data cannot support — absence and never-happened are
	// indistinguishable once the sweep has run.
	got := (&Assembler{}).rung(reg(t, trace.StageInternalDrop),
		view{stored: ladderWith(), owned: true}, nil, nil)
	if got.Status != trace.StatusUnknown {
		t.Errorf("status with no rows = %q, want unknown", got.Status)
	}
	if got.Reason != trace.ReasonRecordNotAvailable {
		t.Errorf("reason = %q, want the record-not-available class", got.Reason)
	}
}

func TestAnInternalDropReportsTheLaterStagesAsNotApplicable(t *testing.T) {
	// Here the rows ARE present and say the message was dropped before the write
	// was attempted, so "did not apply" is exactly right — the mirror of the
	// swept case above, and the reason the two need telling apart.
	stored := ladderWith(row(trace.StageInternalDrop, "internal", "internal_only"))
	got := (&Assembler{}).rung(reg(t, trace.StageActivityWrite),
		view{stored: stored, owned: true}, nil, nil)
	if got.Status != trace.StatusNotApplicable {
		t.Errorf("activity-write status after an internal drop = %q, want not_applicable", got.Status)
	}
}

func TestAHiddenActivityIsNotReportedAsNeverWritten(t *testing.T) {
	// The caller owns the trace row; the activity moved out of their row scope.
	// Saying "did not apply" would contradict the captured rung beside it, and
	// saying "done" would confirm an activity they may not open.
	stored := ladderWith(row(trace.StageTierLadder, "captured", ""))
	v := view{stored: stored, owned: true, activityHidden: true}
	for _, stage := range []trace.Stage{
		trace.StageActivityWrite, trace.StageContactCreate, trace.StageAttentionLabel,
	} {
		got := (&Assembler{}).rung(reg(t, stage), v, nil, nil)
		if got.Status != trace.StatusUnknown || got.Reason != trace.ReasonRecordNotAvailable {
			t.Errorf("%s with a hidden activity = %q/%q, want unknown/record_not_available",
				stage, got.Status, got.Reason)
		}
	}
}

func TestTheAttentionRungCarriesTheReasonActivitiesGaveIt(t *testing.T) {
	// The motivating case. The reason is NOT decided here — activities owns the
	// backlog predicate — so this asserts the rung passes it through unaltered.
	activityID := ids.NewV7()
	stored := ladderWith(row(trace.StageTierLadder, "captured", ""))
	stored.ActivityID = &activityID
	facts := &activities.PipelineFacts{
		ClassifyEligible: false,
		ClassifyReason:   trace.ReasonTransportNotRead,
	}
	got := (&Assembler{}).rung(reg(t, trace.StageAttentionLabel),
		view{stored: stored, owned: true}, facts, nil)
	if got.Status != trace.StatusSkipped {
		t.Errorf("status = %q, want skipped", got.Status)
	}
	if got.Reason != trace.ReasonTransportNotRead {
		t.Errorf("reason = %q, want the transport exclusion activities named", got.Reason)
	}
}

// Each settled ledger status renders as the verdict it IS, and carries the
// moment it was reached.
//
// The rung used to report only that a verdict existed, which is the one fact a
// member opening this panel already had. A status mapped to the wrong sentence
// is invisible to the vocabulary gate — that holds the SET of statuses the fold
// names, not which answer each is — so the pairing is asserted here.
func TestASettledVerdictNamesWhichVerdictItWas(t *testing.T) {
	reached := time.Date(2026, 8, 16, 10, 30, 0, 0, time.UTC)
	for status, want := range map[string]trace.Reason{
		capture.PendingStatusReal:       trace.ReasonJudgedReal,
		capture.PendingStatusNoise:      trace.ReasonJudgedNoise,
		capture.PendingStatusRejected:   trace.ReasonJudgedRejected,
		capture.PendingStatusSuppressed: trace.ReasonJudgedSuppressed,
	} {
		stored := ladderWith(row(trace.StageTierLadder, "deferred", ""))
		stored.Rungs[0].Resolution = &capture.TraceResolution{Status: status, ResolvedAt: &reached}
		got := (&Assembler{}).rung(reg(t, trace.StageVerdict),
			view{stored: stored, owned: true}, nil, nil)
		if got.Status != trace.StatusDone || got.Reason != want {
			t.Errorf("a %q verdict rendered as %q/%q, want %q/%q",
				status, got.Status, got.Reason, trace.StatusDone, want)
		}
		if got.At == nil || !got.At.Equal(reached) {
			t.Errorf("a %q verdict reports %v as the moment it was reached, want %v", status, got.At, reached)
		}
	}
}

// An OPEN status is still a wait, and says so rather than naming a verdict.
func TestAnOpenQuestionIsReportedAsAWaitAndNotAsAVerdict(t *testing.T) {
	for _, status := range []string{capture.PendingStatusPending, capture.PendingStatusUnsure} {
		stored := ladderWith(row(trace.StageTierLadder, "deferred", ""))
		stored.Rungs[0].Resolution = &capture.TraceResolution{Status: status}
		got := (&Assembler{}).rung(reg(t, trace.StageVerdict),
			view{stored: stored, owned: true}, nil, nil)
		if got.Status != trace.StatusPending || got.Reason != trace.ReasonAwaitingVerdict {
			t.Errorf("a %q question rendered as %q/%q, want the wait", status, got.Status, got.Reason)
		}
	}
}

func TestAnUnrecognisedVerdictIsNotReadAsAReachedOne(t *testing.T) {
	// A ledger status a newer binary writes. Reporting it as a reached verdict
	// would tell a member their sender was judged when they were not.
	stored := ladderWith(row(trace.StageTierLadder, "deferred", ""))
	stored.Rungs[0].Resolution = &capture.TraceResolution{Status: "a_state_from_the_future"}
	got := (&Assembler{}).rung(reg(t, trace.StageVerdict),
		view{stored: stored, owned: true}, nil, nil)
	if got.Status == trace.StatusDone {
		t.Errorf("an unrecognised ledger status rendered as %q/%q — a verdict nobody reached",
			got.Status, got.Reason)
	}
}

func TestEveryReasonARungProducesIsOneItsStageDeclared(t *testing.T) {
	// A reason outside the stage's closed set is interpolated into a catalog key
	// at the client and rendered raw at a member — the bug this surface already
	// shipped once, in its own predecessor.
	activityID := ids.NewV7()
	full := ladderWith(
		row(trace.StageInternalDrop, "internal", "internal_only"),
		row(trace.StageActivityWrite, "fault", "invisible_incumbent"),
		row(trace.StageTierLadder, "suppressed", "transactional_infra"),
	)
	full.ActivityID = &activityID
	facts := &activities.PipelineFacts{ClassifyReason: trace.ReasonArchived}

	for _, v := range []view{
		{stored: full, owned: true},
		{stored: full},
		{stored: ladderWith(), owned: true},
		{stored: full, owned: true, activityHidden: true},
	} {
		for _, r := range trace.Registrations() {
			got := (&Assembler{}).rung(r, v, facts, nil)
			if got.Reason == "" || got.Reason == r.AbsentReason {
				continue
			}
			if !declares(r, got.Reason) {
				t.Errorf("%s produced reason %q, which it does not declare", r.Stage, got.Reason)
			}
		}
	}
}

func declares(r trace.Registration, reason trace.Reason) bool {
	for _, declared := range r.Reasons {
		if declared == reason {
			return true
		}
	}
	return false
}

// The conversation rung, over every shape the extractor's rule produces. Each
// case is an arm of that rule — a member reading this rung is reading the
// reason the pass itself acted on, so a rung that answered its own way would be
// the copy this package exists not to be.
func TestMaterialEventsRungAnswersEachArmOfTheOffer(t *testing.T) {
	// offered is a conversation the extractor would read: every arm satisfied,
	// nothing done to it yet. Each case below breaks exactly one thing.
	offered := ThreadReading{
		Known: true, ReachesOneAccount: true, IsOneBodyOfWork: true,
		IsFullyOpen: true, HasANamedReader: true, HasSettled: true, HasMoved: true,
	}
	with := func(edit func(*ThreadReading)) *ThreadReading {
		out := offered
		edit(&out)
		return &out
	}
	emailOnAThread := &activities.PipelineFacts{ThreadKey: "thread-1"}

	for _, c := range []struct {
		name    string
		facts   *activities.PipelineFacts
		reading *ThreadReading
		status  trace.Status
		reason  trace.Reason
	}{
		{
			name:   "a transport with no thread has no unit of work",
			facts:  &activities.PipelineFacts{},
			status: trace.StatusSkipped, reason: trace.ReasonTransportNotRead,
		},
		{
			name:   "no activity means nothing was there to read",
			status: trace.StatusNotApplicable,
		},
		{
			name:  "no composed extractor is not a state to report",
			facts: emailOnAThread, status: trace.StatusUnknown,
		},
		{
			name: "a thread the extractor sees no conversation on", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.Known = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonArchived,
		},
		{
			name: "a signal citing this message", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.Cited, r.Scanned = true, true }),
			status:  trace.StatusDone, reason: trace.ReasonEventsRaised,
		},
		{
			name: "read, and it said nothing about this message", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.Scanned, r.HasMoved = true, false }),
			status:  trace.StatusDone, reason: trace.ReasonNothingMaterial,
		},
		{
			name: "due and not yet read", facts: emailOnAThread, reading: &offered,
			status: trace.StatusPending, reason: trace.ReasonAwaitingScan,
		},
		{
			name: "reaching two accounts", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.ReachesOneAccount = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonNoSingleAccount,
		},
		{
			name: "spanning two bodies of work", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.IsOneBodyOfWork = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonTwoBodiesOfWork,
		},
		{
			name: "carrying a message withheld from the summary's readers", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.IsFullyOpen = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonThreadNotAllOpen,
		},
		{
			name: "with no reader a finding could answer to", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.HasANamedReader = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonNoNamedReader,
		},
		{
			name: "still moving", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.HasSettled = false }),
			status:  trace.StatusSkipped, reason: trace.ReasonThreadStillMoving,
		},
		{
			name: "parked after too many refusals", facts: emailOnAThread,
			reading: with(func(r *ThreadReading) { r.IsParked = true }),
			status:  trace.StatusSkipped, reason: trace.ReasonReadingParked,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			got := (&Assembler{}).rung(reg(t, trace.StageMaterialEvents),
				view{stored: ladderWith(), owned: true}, c.facts, c.reading)
			if got.Status != c.status || got.Reason != c.reason {
				t.Fatalf("rung = %q/%q, want %q/%q", got.Status, got.Reason, c.status, c.reason)
			}
		})
	}
}

// A hidden activity answers about the activity and not about its conversation:
// telling a member which arm a thread they may not read failed is the thread's
// content arriving by another door.
func TestMaterialEventsSaysNothingAboutAConversationTheReaderCannotOpen(t *testing.T) {
	got := (&Assembler{}).rung(reg(t, trace.StageMaterialEvents),
		view{stored: ladderWith(), activityHidden: true},
		&activities.PipelineFacts{ThreadKey: "thread-1"},
		&ThreadReading{Known: true, ReachesOneAccount: false})
	if got.Reason == trace.ReasonNoSingleAccount {
		t.Errorf("the rung named an arm of a conversation the reader may not open: %q", got.Reason)
	}
}
