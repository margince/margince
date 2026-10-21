// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"encoding/json"
	"errors"
	"log/slog"
	"maps"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAutomaticStagePreviewUsesCurrentNamesAndRefusesChangedDestination(t *testing.T) {
	e := Setup(t)
	pipeline, open, _ := DealFixture(t, e)
	deal := ids.From[ids.DealKind](e.SeedDeal(t, "Review expansion", pipeline, open, &e.Rep1))
	label := "The agreement is signed"
	if _, err := e.Deals.CreateStageExitCriterion(e.Admin(), deals.CreateCriterionInput{StageID: open, Key: "signed", Label: label, Kind: string(deals.CriterionDocumentSigned)}); err != nil {
		t.Fatal(err)
	}
	activity, _, err := activities.NewStore(e.DB()).LogActivity(e.Admin(), activities.LogActivityInput{Kind: "note", Subject: new("Signed agreement received"), Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.Deals.RecordDeterministicEvidence(e.Admin(), deals.DeterministicClaim{DealID: deal, Kind: deals.CriterionDocumentSigned, SourceType: "activity", SourceID: ids.UUID(activity.Id), AuthorSide: deals.AuthorBuyer, ObservedAt: stageProgressionClock.Add(-time.Hour)}); err != nil {
		t.Fatal(err)
	}
	proposer := compose.NewStageProgressionProposer(e.Pool, e.Deals, approvals.NewService(e.DB()), func() time.Time { return stageProgressionClock }, slog.New(slog.DiscardHandler))
	if staged, err := proposer.Propose(e.Admin(), deal); err != nil || !staged {
		t.Fatalf("staged=%t err=%v", staged, err)
	}
	var id ids.ApprovalID
	var destination ids.StageID
	if err := e.Pool.QueryRow(t.Context(), `SELECT id, co_target_entity_id FROM approval WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`, deals.StageProgressionKind, deal).Scan(&id, &destination); err != nil {
		t.Fatal(err)
	}
	svc := compose.NewApprovalsService(e.DB())
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" {
		t.Fatalf("preview=%+v", proposal.Preview)
	}
	if proposal.Preview.Details == nil || len(*proposal.Preview.Details) != 1 {
		t.Fatalf("missing criteria: %+v", proposal.Preview)
	}
	criterion := (*proposal.Preview.Details)[0]
	if criterion.Field != "stage_required_criterion_met" || criterion.Value.Text != label {
		t.Fatalf("wrong checklist: %+v", criterion)
	}
	assertUnpinnedProgressionRefused(t, e, proposal)
	stage, err := e.Deals.GetStage(e.Admin(), destination)
	if err != nil {
		t.Fatal(err)
	}
	named := false
	for _, change := range proposal.Preview.Changes {
		if change.Field == "stage_id" && change.After.Text == stage.Name && change.Before.Text != "" {
			named = true
		}
	}
	if !named {
		t.Fatalf("missing named transition: %+v", proposal.Preview.Changes)
	}
	if proposal.Preview.Evidence == nil || len(*proposal.Preview.Evidence) != 1 {
		t.Fatal("preview lost its authorized source")
	}
	if _, err := e.Deals.UpdateStage(e.Admin(), destination, deals.UpdateStageInput{Name: new("Revised destination")}); err != nil {
		t.Fatal(err)
	}
	stale, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Preview == nil || stale.Preview.State != "stale" || len(stale.Preview.Changes) != 0 {
		t.Fatalf("changed destination appears reviewable: %+v", stale.Preview)
	}
	if _, err := svc.Decide(e.Admin(), id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("stale decision: %v", err)
	}
}

func assertProgressionContractPreview(t *testing.T, e *Env, deal ids.DealID, contractID ids.UUID) {
	t.Helper()
	var id ids.ApprovalID
	if err := e.Pool.QueryRow(t.Context(), `SELECT id FROM approval WHERE kind = $1 AND target_entity_id = $2 AND status = 'pending'`, deals.StageProgressionKind, deal).Scan(&id); err != nil {
		t.Fatal(err)
	}
	svc := compose.NewApprovalsService(e.DB())
	proposal, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if proposal.Preview == nil || proposal.Preview.State != "ready" || proposal.Preview.Evidence == nil || len(*proposal.Preview.Evidence) != 1 {
		t.Fatalf("contract preview=%+v", proposal.Preview)
	}
	citation := (*proposal.Preview.Evidence)[0]
	if citation.SourceType == nil || *citation.SourceType != "contract" || citation.SourceId == nil || ids.UUID(*citation.SourceId) != contractID {
		t.Fatalf("wrong contract citation: %+v", citation)
	}
	limited := AdminPerms
	limited.Objects = maps.Clone(limited.Objects)
	limited.Objects["contract"] = principal.ObjectGrant{}
	hidden, err := svc.GetWire(e.As(e.AdminUser, nil, limited), id)
	if err != nil {
		t.Fatal(err)
	}
	if hidden.Preview == nil || hidden.Preview.State != "unavailable" || len(hidden.Preview.Changes) != 0 || (hidden.Preview.Evidence != nil && len(*hidden.Preview.Evidence) != 0) {
		t.Fatalf("unreadable contract disclosed: %+v", hidden.Preview)
	}
	evidence, err := e.Deals.ListStageEvidence(e.Admin(), deal)
	if err != nil || len(evidence) != 1 {
		t.Fatalf("evidence=%v err=%v", evidence, err)
	}
	if _, err := e.Deals.RefuteStageEvidence(e.Admin(), deal, ids.UUID(evidence[0].Id)); err != nil {
		t.Fatal(err)
	}
	stale, err := svc.GetWire(e.Admin(), id)
	if err != nil {
		t.Fatal(err)
	}
	if stale.Preview == nil || stale.Preview.State != "stale" || len(stale.Preview.Changes) != 0 {
		t.Fatalf("refuted evidence accepted: %+v", stale.Preview)
	}
	if _, err := svc.Decide(e.Admin(), id, true, nil); !errors.Is(err, apperrors.ErrVersionSkew) {
		t.Fatalf("refuted decision: %v", err)
	}
}

func assertProgressionAppliedOnce(t *testing.T, e *Env, deal ids.DealID, card ids.ApprovalID) {
	t.Helper()
	var current, target ids.UUID
	var version, pinned int64
	var consumed bool
	if err := e.Pool.QueryRow(t.Context(), `SELECT d.stage_id, a.co_target_entity_id, d.version, a.target_version, a.consumed_at IS NOT NULL
 FROM deal d JOIN approval a ON a.target_entity_id = d.id WHERE d.id = $1 AND a.id = $2`, deal, card).Scan(&current, &target, &version, &pinned, &consumed); err != nil {
		t.Fatal(err)
	}
	if current != target || version != pinned+1 || !consumed {
		t.Fatalf("stage=%v target=%v version=%d pin=%d consumed=%t", current, target, version, pinned, consumed)
	}
	if _, err := compose.StageProgressionDecisions(e.Pool).Decide(e.Admin(), card, true, nil); err != nil {
		if _, ok := errors.AsType[*approvals.AlreadyDecidedError](err); !ok {
			t.Fatalf("replay: %v", err)
		}
	}
	var after int64
	if err := e.Pool.QueryRow(t.Context(), `SELECT version FROM deal WHERE id = $1`, deal).Scan(&after); err != nil {
		t.Fatal(err)
	}
	if after != version {
		t.Fatal("replayed approval applied another change")
	}
}

func TestAutomaticMoveWaitsForReadableEvidence(t *testing.T) {
	e := Setup(t)
	deal, staged := proposeOnMetCriteria(t, e)
	if !staged {
		t.Fatal("no staged proposal")
	}
	governedTransition(t, e, deal)
	e.WsExec(t, `UPDATE role SET permissions = jsonb_set(permissions, '{objects,contract,read}', 'false') WHERE key = 'stage-autopilot-rep'`)
	before := stageOf(t, e, deal)
	applied, err := compose.SweepAutoApply(e.Admin(), e.Pool)
	if err != nil || applied != 0 {
		t.Fatalf("unreadable evidence: applied=%d err=%v", applied, err)
	}
	if stageOf(t, e, deal) != before {
		t.Fatal("unreadable evidence moved the deal")
	}
	if _, pending := pendingCardFor(t, e, deal); !pending {
		t.Fatal("unreadable evidence consumed the pending decision")
	}
}

func assertUnpinnedProgressionRefused(t *testing.T, e *Env, original crmcontracts.Approval) {
	t.Helper()
	raw, err := json.Marshal(original.ProposedChange)
	if err != nil {
		t.Fatal(err)
	}
	canonical, hash, err := diffhash.Canonical(raw)
	if err != nil {
		t.Fatal(err)
	}
	svc := compose.NewApprovalsService(e.DB())
	legacy, err := svc.Stage(e.Admin(), approvals.StageInput{Kind: deals.StageProgressionKind, TargetType: "deal", TargetID: ids.UUID(*original.TargetEntityId), ProposedChange: canonical, DiffHash: hash})
	if err != nil {
		t.Fatal(err)
	}
	shown, err := svc.GetWire(e.Admin(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if shown.Preview == nil || shown.Preview.State != "incomplete" || len(shown.Preview.Changes) != 0 || shown.Preview.Effects != nil {
		t.Fatalf("legacy row appears reviewable: %+v", shown.Preview)
	}
	var missing *approvals.PreviewContextError
	if _, err := svc.Decide(e.Admin(), legacy, true, nil); !errors.As(err, &missing) || missing.State != "incomplete" {
		t.Fatalf("legacy decision: %v", err)
	}
	after, err := svc.GetWire(e.Admin(), legacy)
	if err != nil {
		t.Fatal(err)
	}
	if after.Status != "pending" {
		t.Fatalf("refused legacy decision became %s", after.Status)
	}
}
