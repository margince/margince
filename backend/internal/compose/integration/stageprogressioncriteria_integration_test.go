// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestStageApprovalRefusesAChangedChecklist(t *testing.T) {
	for _, mutation := range []string{"rename", "kind", "add"} {
		t.Run(mutation, func(t *testing.T) {
			e := Setup(t)
			deal, staged := proposeOnMetCriteria(t, e)
			if !staged {
				t.Fatal("no proposal")
			}
			id, pending := pendingCardFor(t, e, deal)
			if !pending {
				t.Fatal("no pending decision")
			}
			svc := compose.StageProgressionDecisions(e.Pool)
			original, err := svc.GetWire(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if original.Preview == nil || original.Preview.State != "ready" {
				t.Fatalf("initial preview=%+v", original.Preview)
			}
			stage := stageOf(t, e, deal)
			criteria, err := e.Deals.ListStageExitCriteria(e.Admin(), stage, storekit.LiveOnly)
			if err != nil || len(criteria) == 0 {
				t.Fatalf("criteria=%v err=%v", criteria, err)
			}
			switch mutation {
			case "add":
				_, err = e.Deals.CreateStageExitCriterion(e.Admin(), deals.CreateCriterionInput{StageID: stage, Key: "acceptance", Label: "Buyer accepted the terms", Kind: string(deals.CriterionTermsAccepted)})
			default:
				edit := deals.UpdateCriterionInput{Label: new("Buyer signed the final agreement")}
				if mutation == "kind" {
					edit = deals.UpdateCriterionInput{Kind: new(string(deals.CriterionTermsAccepted))}
				}
				_, err = e.Deals.UpdateStageExitCriterion(e.Admin(), stage, ids.From[ids.ExitCriterionKind](ids.UUID(criteria[0].Id)), edit)
			}
			if err != nil {
				t.Fatal(err)
			}
			stale, err := svc.GetWire(e.Admin(), id)
			if err != nil {
				t.Fatal(err)
			}
			if stale.Preview == nil || stale.Preview.State != "stale" || len(stale.Preview.Changes) != 0 {
				t.Fatalf("changed checklist appears reviewable: %+v", stale.Preview)
			}
			if _, err := svc.Decide(e.Admin(), id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
				t.Fatalf("decision=%v", err)
			}
			if stageOf(t, e, deal) != stage {
				t.Fatal("refused decision moved the deal")
			}
			if _, pending := pendingCardFor(t, e, deal); !pending {
				t.Fatal("refused decision consumed the proposal")
			}
		})
	}
}

func TestLegacyStageChecklistNeedsRestaging(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("no proposal")
	}
	id, pending := pendingCardFor(t, e, deal)
	if !pending {
		t.Fatal("no pending decision")
	}
	svc := compose.StageProgressionDecisions(e.Pool)
	original, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(original.ProposedChange)
	if err != nil {
		t.Fatal(err)
	}
	change, err := deals.ReadProgressionChange(raw)
	if err != nil {
		t.Fatal(err)
	}
	for i := range change.Criteria {
		change.Criteria[i].Version = 0
	}
	raw, err = json.Marshal(change)
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	legacy, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: deals.StageProgressionKind, TargetType: "deal", TargetID: deal.UUID, CoTargetType: "stage", CoTargetID: change.ToStageID.UUID, ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := svc.GetWire(e.Admin(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Preview == nil || shown.Preview.State != "incomplete" || shown.Preview.Details != nil {
		t.Fatalf("legacy checklist remains reviewable: %+v", shown.Preview)
	}
	var missing *approvals.PreviewContextError
	if _, err := svc.Decide(e.Admin(), legacy, true, nil); !errors.As(err, &missing) || missing.State != "incomplete" {
		t.Fatalf("legacy decision: %v", err)
	}
	if stageOf(t, e, deal) != change.FromStageID {
		t.Fatal("legacy decision moved the deal")
	}
}

func TestChecklistChangeAfterPreviewRefusesTheRegisteredEffect(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("no proposal")
	}
	id, pending := pendingCardFor(t, e, deal)
	if !pending {
		t.Fatal("no pending decision")
	}
	before := stageOf(t, e, deal)
	svc := compose.StageProgressionDecisions(e.Pool)
	changed := false
	svc.WithPrecheck(deals.StageProgressionKind, func(ctx context.Context, _, _ json.RawMessage) error {
		_, err := e.Deals.CreateStageExitCriterion(ctx, deals.CreateCriterionInput{StageID: before, Key: "late_acceptance", Label: "Buyer accepted the final terms", Kind: string(deals.CriterionTermsAccepted)})
		changed = err == nil
		return err
	})
	if _, err := svc.Decide(e.Admin(), id, true, nil); err == nil {
		t.Fatal("a checklist changed after preview still moved the deal")
	}
	if !changed {
		t.Fatal("the test never reached the interval after preview")
	}
	if stageOf(t, e, deal) != before {
		t.Fatal("the registered effect moved the deal on stale criteria")
	}
	var consumed bool
	var failure *string
	if err := e.Pool.QueryRow(t.Context(), `SELECT consumed_at IS NOT NULL, effect_failure FROM approval WHERE id = $1`, id).Scan(&consumed, &failure); err != nil {
		t.Fatal(err)
	}
	if consumed || failure == nil {
		t.Fatalf("consumed=%t failure=%v", consumed, failure)
	}
}
