// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/platform/approvalsubject"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func stageCounterpartyReview(ctx context.Context, svc *approvals.Service, said approvalSummaryCopy, row capture.PendingCounterparty) (ids.UUID, error) {
	proposal := approvalsubject.Counterparty{
		DispositionID: row.ID,
		Email:         row.Email,
		DisplayName:   row.DisplayName,
		Domain:        row.Domain,
		OwnerID:       row.OwnerID,
		ActivityID:    row.ActivityID,
	}
	body, err := json.Marshal(proposal)
	if err != nil {
		return ids.Nil, fmt.Errorf("compose: encoding the counterparty proposal: %w", err)
	}
	digest := sha256.Sum256(body)
	id, err := svc.Stage(ctx, approvals.StageInput{
		Kind:           counterpartyProposalKind,
		ProposedChange: body,
		DiffHash:       hex.EncodeToString(digest[:]),
		TargetType:     counterpartyTargetType,
		TargetID:       row.ActivityID,
		TargetLabel:    counterpartyLabel(row),
		Summary:        fmt.Sprintf(said.counterpartyWorthKeeping, row.Email),
		// The verdict pass is an at-least-once worker: a retried batch must
		// return the existing offer rather than multiply inbox rows.
		JoinPending: true,
	})
	if err != nil {
		return ids.Nil, err
	}
	return id.UUID, nil
}

// counterpartyLabel names the sender a counterparty proposal is about, so the
// receipt and the inbox say who wrote rather than which message.
func counterpartyLabel(row capture.PendingCounterparty) *string {
	label := row.Email
	if name := strings.TrimSpace(row.DisplayName); name != "" {
		label = name + " <" + row.Email + ">"
	}
	return &label
}

// Historical approvals exercise the existing redemption contract after automatic staging stops.
func seedLegacySenderReviews(t *testing.T, e *integration.Env, engine *CounterpartyVerdictEngine) error {
	t.Helper()
	return engine.inWorkspace(principal.WithWorkspaceID(context.Background(), e.WS), func(ctx context.Context, _ ids.UUID) error {
		rows, err := engine.pending.AwaitingReview(ctx, 100)
		if err != nil {
			return err
		}
		for _, row := range rows {
			id, err := stageCounterpartyReview(ctx, engine.approvals, approvalSummaryByLang["en"], row)
			if err != nil {
				return err
			}
			if err := engine.pending.LinkProposal(ctx, row.ID, id); err != nil {
				return err
			}
		}
		return nil
	})
}

func TestTheSweepRetiresLegacyUncertaintyOffersWithoutRestaging(t *testing.T) {
	e := integration.Setup(t)
	activity := seedCapturedMail(t, e, "old@uncertain.test", "Hello")
	disposition := seedPendingDisposition(t, e, "old@uncertain.test", "uncertain.test", activity)
	retireToUnsure(t, e, disposition)
	engine := NewCounterpartyVerdictEngine(e.Pool, &scriptedVerdictBrain{}, CaptureConfig{}, nil)
	if err := seedLegacySenderReviews(t, e, engine); err != nil {
		t.Fatal(err)
	}
	proposal := stagedProposalID(t, e, disposition)
	for range 2 {
		if err := engine.StageReviewsWorkspace(e.Admin(), 0); err != nil {
			t.Fatal(err)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM approval WHERE id=$1 AND status='pending'`, proposal); got != 0 {
		t.Fatal("the obsolete uncertainty offer remains actionable")
	}
	if got := counterpartyProposals(t, e, disposition); got != 1 {
		t.Fatalf("the sweep created %d offers", got)
	}
	if _, err := NewApprovalsServiceWithEffects(e.Pool).Decide(e.As(e.Rep1, nil, integration.AdminPerms), ids.From[ids.ApprovalKind](proposal), true, nil); err == nil {
		t.Fatal("the retired uncertainty offer was redeemed")
	}
	assertSenderReviewCount(t, e, e.Rep1, 0)
}
