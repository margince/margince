// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The did_not_run lane: a decision this reader approved whose released work
// then failed comes back to them, with the recorded sentence and — when the
// decision named a record — a way to open it.

import (
	"context"
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type stubFailedEffects struct {
	rows []FailedEffect
	err  error
}

func (s *stubFailedEffects) Failed(context.Context, int) ([]FailedEffect, error) {
	return s.rows, s.err
}

func failedLaneService(failed FailedEffects) *Service {
	return NewService(
		stubApprovals{}, stubDuplicates{}, &stubTasks{}, stubReceipts{},
		stubBriefing{}, nil, nil, nil, nil, failed, fixedClock)
}

func TestAFailedDecisionComesBackToItsDecider(t *testing.T) {
	person := ids.NewV7()
	svc := failedLaneService(&stubFailedEffects{rows: []FailedEffect{{
		ID: ids.NewV7(), Kind: "send_email",
		Sentence:   "this was approved, but the work it released did not run",
		FailedAt:   readInstant,
		TargetType: "person", TargetID: person,
	}, {
		ID: ids.NewV7(), Kind: "quota_release",
		Sentence: "the agent's window could not be widened, so the approval has not taken effect",
		FailedAt: readInstant,
	}}})

	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	if out.DidNotRun == nil {
		t.Fatal("the lane is absent although the feed reads failure marks")
	}
	items := *out.DidNotRun
	if len(items) != 2 || out.Counts.DidNotRun == nil || *out.Counts.DidNotRun != 2 {
		t.Fatalf("lane carries %d items (count %v), want 2 and 2", len(items), out.Counts.DidNotRun)
	}
	targeted := items[0]
	if targeted.Title == nil || *targeted.Title != "this was approved, but the work it released did not run" {
		t.Errorf("the card's title = %v, want the recorded sentence", targeted.Title)
	}
	if targeted.Subject == nil || targeted.Subject.Type != "person" || !slices.Contains(targeted.Actions, "open") {
		t.Errorf("a failure about a named record must offer open on it: %+v", targeted)
	}
	// The second decision named no record, so the card points nowhere rather
	// than guessing — and must not claim it can.
	untargeted := items[1]
	if untargeted.Subject != nil || slices.Contains(untargeted.Actions, "open") {
		t.Errorf("a failure naming no record offered navigation: %+v", untargeted)
	}
}

// An activity is a timeline entry with no page of its own: the card may name
// it, but offering `open` on it would be a link the client cannot route.
func TestAFailureAboutATimelineEntryNamesItWithoutOfferingOpen(t *testing.T) {
	svc := failedLaneService(&stubFailedEffects{rows: []FailedEffect{{
		ID: ids.NewV7(), Kind: "send_email",
		Sentence: "this was approved, but the work it released did not run",
		FailedAt: readInstant, TargetType: "activity", TargetID: ids.NewV7(),
	}}})
	out, err := svc.Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling: %v", err)
	}
	item := (*out.DidNotRun)[0]
	if item.Subject == nil || item.Subject.Type != "activity" {
		t.Fatalf("the card lost its subject: %+v", item)
	}
	if slices.Contains(item.Actions, "open") {
		t.Error("open was offered on a timeline entry no screen answers to")
	}
}

// Absent versus withheld versus empty are three different answers, and each
// must survive as itself: no reader wired means no lane on the wire, a
// refusal names the lane, and a clear lane is an empty list.
func TestTheFailedLaneKeepsAbsentWithheldAndEmptyApart(t *testing.T) {
	unwired, err := failedLaneService(nil).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling without the reader: %v", err)
	}
	if unwired.DidNotRun != nil || unwired.Counts.DidNotRun != nil {
		t.Error("an installation that reads no failure marks still sent the lane")
	}

	refused, err := failedLaneService(&stubFailedEffects{err: apperrors.ErrPermissionDenied}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling with a refused read: %v", err)
	}
	if refused.LanesOmitted == nil || !slices.Contains(*refused.LanesOmitted, crmcontracts.AttentionLanesOmitted("did_not_run")) {
		t.Errorf("a refused lane is not named in lanes_omitted: %v", refused.LanesOmitted)
	}

	clearDay, err := failedLaneService(&stubFailedEffects{}).Assemble(context.Background())
	if err != nil {
		t.Fatalf("assembling a clear lane: %v", err)
	}
	if clearDay.DidNotRun == nil || len(*clearDay.DidNotRun) != 0 {
		t.Errorf("a clear lane should be an empty list, got %v", clearDay.DidNotRun)
	}
}
