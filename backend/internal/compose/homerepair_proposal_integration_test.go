// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestReviewedHistoricalProposalWithdrawsWithoutRecreatingWork(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	old := now.AddDate(0, 0, -90)
	commitment, _ := stageRepairProposal(t, e, old)
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	writer := principal.SystemActing(e.Admin(), "system:repair-fixture")
	operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
	var reviewed datatask.ReviewRequest
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		var err error
		reviewed, err = previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	var scope homeRepairScope
	if err := json.Unmarshal(reviewed.Scope, &scope); err != nil {
		t.Fatal(err)
	}
	if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairWithdrawProposal || scope.Tasks[0].Snapshot == "" {
		t.Fatalf("preview=%+v", scope)
	}
	if e.WsCount(t, "SELECT count(*) FROM approval WHERE status='pending'") != 1 {
		t.Fatal("preview mutated proposal")
	}
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		if _, err := validateHomeRepairTx(operator, tx, reviewed); err != nil {
			return err
		}
		changed, err := applyHomeRepairProposal(operator, tx, scope.Tasks[0], datatask.Activation{ReviewRequest: reviewed}, ids.NewV7())
		if err == nil && !changed {
			t.Fatal("reviewed proposal was not withdrawn")
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	commitment.AsOf = now
	if err := e.DB().Tx(writer, func(tx pgx.Tx) error {
		_, _, err := dispatcher.DispatchTx(writer, tx, commitment, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM approval") != 1 || e.WsCount(t, "SELECT count(*) FROM approval WHERE status='expired'") != 1 || e.WsCount(t, "SELECT count(*) FROM activity WHERE kind='task'") != 0 {
		t.Fatal("another extraction recreated withdrawn automatic work")
	}
}

// stageRepairProposal stages a proposal on imported evidence, which the horizon retires.
func stageRepairProposal(t *testing.T, e *integration.Env, at time.Time) (Commitment, ids.UUID) {
	t.Helper()
	return stageRepairProposalFrom(t, e, at, "import")
}

func stageRepairProposalFrom(t *testing.T, e *integration.Env, at time.Time, logged string) (Commitment, ids.UUID) {
	t.Helper()
	body := "I will prepare the account review."
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Body: &body, Source: logged, OccurredAt: &at})
	if err != nil {
		t.Fatal(err)
	}
	sourceID := ids.UUID(source.Id)
	commitment := Commitment{Meaning: openCommitmentMeaning("", body), AsOf: at.Add(time.Hour), ReadMessages: []ids.UUID{sourceID}, Extractor: signalScanActor, SourceActivityID: sourceID, Summary: "Prepare the account review", Quote: body, Seat: &e.Rep1, Confidence: 0.95, Locator: commitmentLocator(sourceID, claimKindOurs, e.Rep1.String(), body), Body: body, NeedsConfirmation: true, Evidence: approvals.Evidence{Snippet: body, SourceType: "activity", SourceID: sourceID}}
	dispatcher := NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB()))
	writer := principal.SystemActing(e.Admin(), "system:repair-fixture")
	var proposalID ids.UUID
	if err := e.DB().Tx(writer, func(tx pgx.Tx) error {
		outcome, id, err := dispatcher.DispatchTx(writer, tx, commitment, ids.NewV7())
		proposalID = id
		if err == nil && outcome != CommitmentProposed {
			t.Fatalf("fixture outcome=%s", outcome)
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}

	return commitment, proposalID
}

func TestReviewedDuplicateProposalKeepsTheOriginalAndDoesNotRestage(t *testing.T) {
	e := integration.Setup(t)
	at := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	commitment, originalID := stageRepairProposal(t, e, at)
	operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
	writer := onBehalfOf(principal.SystemActing(e.Admin(), "system:repair-fixture"), e.Rep1)
	service := approvals.NewService(e.DB())
	original, err := service.Get(operator, ids.From[ids.ApprovalKind](originalID))
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(original.ProposedChange)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := service.Stage(writer, approvals.StageInput{Kind: CommitmentTaskKind, TargetType: "activity", TargetID: commitment.SourceActivityID, ProposedChange: canonical, DiffHash: hash, Summary: commitment.Summary, Evidence: []approvals.Evidence{commitment.Evidence}})
	if err != nil {
		t.Fatal(err)
	}
	assertRepairDecisionProjection(operator, t, e, at.Add(time.Hour), 2)
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		review, err := previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, at.Add(time.Hour))
		if err != nil {
			return err
		}
		var scope homeRepairScope
		if err := json.Unmarshal(review.Scope, &scope); err != nil {
			return err
		}
		if len(scope.Tasks) != 2 {
			t.Fatalf("review=%+v", scope)
		}
		changes := 0
		for _, item := range scope.Tasks {
			if item.ID == duplicate.UUID && (item.Action != repairWithdrawProposal || item.Reason != "duplicate_pending_proposal") {
				t.Fatalf("duplicate=%+v", item)
			}
			if item.ID == originalID && item.Action != repairLeaveCurrent {
				t.Fatalf("original=%+v", item)
			}
			changed, err := applyHomeRepairProposal(operator, tx, item, datatask.Activation{ReviewRequest: review}, ids.NewV7())
			if err != nil {
				return err
			}
			if changed {
				changes++
			}
		}
		if changes != 1 {
			t.Fatalf("changed=%d", changes)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	assertRepairDecisionProjection(operator, t, e, at.Add(time.Hour), 1)
	dispatcher := NewCommitmentDispatcher(e.Pool, service)
	if err := e.DB().Tx(writer, func(tx pgx.Tx) error {
		_, _, err := dispatcher.DispatchTx(writer, tx, commitment, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if e.WsCount(t, "SELECT count(*) FROM approval") != 2 || e.WsCount(t, "SELECT count(*) FROM approval WHERE status='pending'") != 1 {
		t.Fatal("duplicate repair lost the original or restaged noise")
	}
}

func TestReviewedProposalRepairPreservesLaterHumanDecisionsAndSourcePrivacy(t *testing.T) {
	for _, change := range []string{"decided", "source_hidden"} {
		t.Run(change, func(t *testing.T) {
			e := integration.Setup(t)
			now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
			commitment, proposalID := stageRepairProposal(t, e, now.AddDate(0, 0, -90))
			operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
			var review datatask.ReviewRequest
			if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
				var err error
				review, err = previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, now)
				return err
			}); err != nil {
				t.Fatal(err)
			}
			var scope homeRepairScope
			if err := json.Unmarshal(review.Scope, &scope); err != nil {
				t.Fatal(err)
			}
			if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairWithdrawProposal {
				t.Fatalf("review=%+v", scope)
			}
			if change == "decided" {
				if _, err := approvals.NewService(e.DB()).Decide(operator, ids.From[ids.ApprovalKind](proposalID), false, nil); err != nil {
					t.Fatal(err)
				}
			} else {
				if _, err := e.Activities.SetAudience(e.Admin(), ids.From[ids.ActivityKind](commitment.SourceActivityID), activities.SetAudienceInput{Audience: "selected", Members: []activities.AudienceMember{{SubjectType: "user", SubjectID: e.Rep2}}}); err != nil {
					t.Fatal(err)
				}
			}
			if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
				changed, err := applyHomeRepairProposal(operator, tx, scope.Tasks[0], datatask.Activation{ReviewRequest: review}, ids.NewV7())
				if changed {
					t.Fatal("repair overwrote a human decision or used a hidden source")
				}
				return err
			}); err != nil {
				t.Fatal(err)
			}
			if e.WsCount(t, "SELECT count(*) FROM approval WHERE status='expired'") != 0 {
				t.Fatal("repair withdrew a changed proposal")
			}
		})
	}
}

func TestReviewedRepairLeavesAHandLoggedProposalPastTheHorizon(t *testing.T) {
	e := integration.Setup(t)
	now := time.Date(2026, 4, 1, 12, 0, 0, 0, time.UTC)
	stageRepairProposalFrom(t, e, now.AddDate(0, 0, -90), "manual")
	operator := e.As(e.Rep1, nil, integration.AdminWithSignals)
	if err := e.DB().Tx(operator, func(tx pgx.Tx) error {
		review, err := previewHomeRepairSourceTx(operator, tx, "approval", ids.Nil, 10, now)
		if err != nil {
			return err
		}
		var scope homeRepairScope
		if err := json.Unmarshal(review.Scope, &scope); err != nil {
			return err
		}
		if len(scope.Tasks) != 1 || scope.Tasks[0].Action != repairLeaveCurrent || scope.Tasks[0].Reason != string(CommitmentHistoricalProposed) {
			t.Fatalf("review=%+v, want the late proposal left for its promiser", scope)
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
