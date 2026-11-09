// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestAgentStagingRequiresTheEffectsObjectGrant(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	in := e.mergeCall(e.seedTag(t, "source"), e.seedTag(t, "target"))
	ctx := e.asPassport(passport)
	actor, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no proposer")
	}
	actor.Permissions = grantsFor(map[string]principal.ObjectGrant{"tag": {Read: true}})
	ctx, recorded := auth.RecordChecksForTest(principal.WithActor(ctx, actor))
	if _, _, err := e.svc.StageAgentCall(ctx, in); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("read-only proposer staged a merge: %v", err)
	}
	if got := recorded(); !slices.Contains(got.Refused, "tag.update") {
		t.Fatalf("the staging refusal was not recorded: %+v", got)
	}
	var count int
	if err := e.owner.QueryRow(ctx, `SELECT count(*) FROM approval`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("refused call left %d approvals", count)
	}
	actor.Permissions.Objects["tag"] = principal.ObjectGrant{Read: true, Update: true}
	if _, _, err := e.svc.StageAgentCall(principal.WithActor(ctx, actor), in); err != nil {
		t.Fatal(err)
	}
}

func TestRevokedProposerAuthorityLeavesTheApprovalPending(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	ctx := e.asPassport(passport)
	proposer, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no proposer")
	}
	in := e.mergeCall(e.seedTag(t, "source"), e.seedTag(t, "target"))
	id, _, err := e.svc.StageAgentCall(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	e.svc.WithEffectActor(func(context.Context, pgx.Tx, StagedBy) (principal.Principal, error) { return proposer, nil })
	proposer.Permissions = grantsFor(map[string]principal.ObjectGrant{"tag": {Read: true}})
	e.svc.WithPreview(func(context.Context, crmcontracts.Approval) (*crmcontracts.ApprovalPreview, error) {
		return &crmcontracts.ApprovalPreview{Version: 2, State: "ready", Changes: []crmcontracts.ApprovalPreviewChange{}}, nil
	})
	if _, err := e.svc.Decide(e.asHumanWith(grantsFor(map[string]principal.ObjectGrant{"tag": {Read: true, Update: true}})), id, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("decision ignored revoked proposer grant: %v", err)
	}
	var status string
	var consumed bool
	if err := e.owner.QueryRow(ctx, `SELECT status, consumed_at IS NOT NULL FROM approval WHERE id = $1`, id).Scan(&status, &consumed); err != nil {
		t.Fatal(err)
	}
	if status != statusPending || consumed {
		t.Fatalf("refusal changed approval: %s", status)
	}
}

func TestAgentStagePrecheckRunsAgainBeforeDecision(t *testing.T) {
	e := setupStaging(t)
	passport := e.seedPassport(t)
	ctx := e.asPassport(passport)
	proposer, ok := principal.Actor(ctx)
	if !ok {
		t.Fatal("no proposer")
	}
	e.svc.WithEffectActor(func(context.Context, pgx.Tx, StagedBy) (principal.Principal, error) { return proposer, nil })
	refused := false
	e.svc.WithStagePrecheck("enrich", func(context.Context, StageInput) error {
		if refused {
			return apperrors.ErrConsentNotGranted
		}
		return nil
	})
	id, _, err := e.svc.StageAgentCall(ctx, e.agentCall(e.seedCompany(t)))
	if err != nil {
		t.Fatal(err)
	}
	refused = true
	if _, err := e.svc.Decide(e.asHumanWith(decidesEverything()), id, true, nil); !errors.Is(err, apperrors.ErrConsentNotGranted) {
		t.Fatalf("decision ignored the live precheck: %v", err)
	}
}

func TestAgentStagingRefusesMissingOperandGrants(t *testing.T) {
	for _, tc := range []struct {
		kind, target string
		objects      map[string]principal.ObjectGrant
	}{
		{"promote_lead", "lead", map[string]principal.ObjectGrant{"lead": {Read: true, Update: true}, "contact": {Read: true}}},
		{"merge_records", "contact", map[string]principal.ObjectGrant{"contact": {Read: true}, "company": {Read: true, Update: true}}},
	} {
		t.Run(tc.kind, func(t *testing.T) {
			e := setupStaging(t)
			ctx := e.asPassport(e.seedPassport(t))
			actor, ok := principal.Actor(ctx)
			if !ok {
				t.Fatal("no proposer")
			}
			actor.Permissions = grantsFor(tc.objects)
			_, _, err := e.svc.StageAgentCall(principal.WithActor(ctx, actor), StageInput{Kind: tc.kind, TargetType: tc.target, TargetID: ids.NewV7(), ProposedChange: []byte(`{}`), DiffHash: "missing-operand"})
			if !errors.Is(err, apperrors.ErrPermissionDenied) {
				t.Fatalf("missing operand grant staged: %v", err)
			}
			if count := e.count(t, `SELECT count(*) FROM approval`); count != 0 {
				t.Fatalf("wrote %d approvals", count)
			}
		})
	}
}

func TestAnAgentApprovalCannotBeDecidedWithoutResolvingItsProposer(t *testing.T) {
	e := setupStaging(t)
	id, _, err := e.svc.StageAgentCall(e.asPassport(e.seedPassport(t)), e.agentCall(e.seedCompany(t)))
	if err != nil {
		t.Fatal(err)
	}
	e.svc.WithEffectActor(nil)
	if _, err := e.svc.Decide(e.asDecider(), id, true, nil); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("unresolved proposer was elevated to the decider: %v", err)
	}
	if e.statusOf(t, id) != statusPending {
		t.Fatal("refusal spent the approval")
	}
}

func TestAnAgentCallCannotMintAHumanAgreement(t *testing.T) {
	e := setupStaging(t)
	in := e.agentCall(e.seedCompany(t))
	in.Kind = approvalsubject.KindOwnershipTransfer
	other := ids.NewV7()
	body, err := json.Marshal(approvalsubject.OwnershipTransfer{
		RecordType: in.TargetType, RecordID: in.TargetID, FromID: e.rep, ToID: other, RequestedBy: e.rep, DeciderID: other,
	})
	if err != nil {
		t.Fatal(err)
	}
	in.ProposedChange = body
	_, _, err = e.svc.StageAgentCall(e.asPassport(e.seedPassport(t)), in)
	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("agent minted a human agreement: %v", err)
	}
	if count := e.count(t, `SELECT count(*) FROM approval`); count != 0 {
		t.Fatalf("refused call left %d approvals", count)
	}
}
