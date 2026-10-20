// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type settledCommitmentIdentity struct {
	sourceID     ids.UUID
	locator      string
	claimID      *ids.UUID
	status       string
	settlementID ids.UUID
	taskID       *ids.UUID
}

func (t *CommitmentSettleTrigger) withdrawSettledOffers(ctx context.Context, settled settledCommitmentIdentity) error {
	err := t.retireSettledOffers(ctx, settled)
	if refused(err) {
		t.log.WarnContext(ctx, "settled commitment no longer accessible", "source", settled.sourceID, "err", err)
		return nil
	}
	return err
}

func (t *CommitmentSettleTrigger) retireSettledOffers(ctx context.Context, settled settledCommitmentIdentity) error {
	if settled.sourceID.IsZero() || strings.TrimSpace(settled.locator) == "" {
		return nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	reason := "The promise has been completed"
	if settled.status == claimStatusDismissed {
		reason = "The promise was dismissed"
	}
	// Keep the staging lock order: source, proposals, then the settled record.
	return t.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := activities.LockActivityLive(ctx, tx, ids.From[ids.ActivityKind](settled.sourceID)); err != nil {
			return err
		}
		if err := auth.EnsureActivityContentVisibleLive(ctx, tx, settled.sourceID); err != nil {
			return err
		}
		offers, err := t.approvals.OffersForTx(ctx, tx, CommitmentTaskKind, settled.sourceID)
		if err != nil {
			return err
		}
		current, err := t.stillSettledTx(ctx, tx, settled)
		if err != nil || !current {
			return err
		}
		return t.withdrawMatchingOffers(ctx, tx, settled, offers, reason)
	})
}

func (t *CommitmentSettleTrigger) withdrawMatchingOffers(ctx context.Context, tx pgx.Tx, settled settledCommitmentIdentity, offers []approvals.Offer, reason string) error {
	for _, offer := range offers {
		if !offer.Pending() {
			continue
		}
		var proposal CommitmentTaskProposal
		if err := json.Unmarshal(offer.Change, &proposal); err != nil {
			return fmt.Errorf("read commitment offer identity: %w", err)
		}
		if proposal.SourceActivityID != settled.sourceID || proposal.Locator != settled.locator {
			continue
		}
		if _, err := t.approvals.WithdrawInTx(ctx, tx, offer.ID, reason); err != nil {
			return err
		}
	}
	return nil
}

func (t *CommitmentSettleTrigger) withdrawCompletedTaskOffers(ctx context.Context, task crmcontracts.Activity) error {
	if task.Kind != bulkParamTask || task.IsDone == nil || !*task.IsDone || task.SourceActivityId == nil ||
		task.SourceSystem == nil || *task.SourceSystem != activities.CommitmentTaskSource || task.SourceId == nil {
		return nil
	}
	return t.withdrawSettledOffers(ctx, settledCommitmentIdentity{sourceID: ids.UUID(*task.SourceActivityId), locator: *task.SourceId, status: claimStatusDone, taskID: new(ids.UUID(task.Id))})
}

func (t *CommitmentSettleTrigger) stillSettledTx(ctx context.Context, tx pgx.Tx, prior settledCommitmentIdentity) (bool, error) {
	if prior.claimID != nil {
		return t.claimStillSettledTx(ctx, tx, prior)
	}
	if prior.taskID != nil {
		current, err := t.tasks.LockTaskForBulkTx(ctx, tx, ids.From[ids.ActivityKind](*prior.taskID))
		if err != nil {
			return false, err
		}
		if !current.Done || current.SourceSystem == nil || *current.SourceSystem != activities.CommitmentTaskSource ||
			current.SourceID == nil || *current.SourceID != prior.locator ||
			current.SourceActivityID == nil || *current.SourceActivityID != prior.sourceID {
			return false, nil
		}
		return t.recipientsSettledTx(ctx, tx, prior.taskID, prior.sourceID, prior.locator)
	}
	return false, nil
}

func (t *CommitmentSettleTrigger) claimStillSettledTx(ctx context.Context, tx pgx.Tx, prior settledCommitmentIdentity) (bool, error) {
	current, err := t.claims.ReadClaimSettlementTx(ctx, tx, *prior.claimID, prior.settlementID)
	if err != nil || current == nil || current.Status != prior.status || current.SourceID != prior.sourceID || current.Locator != prior.locator {
		return false, err
	}
	return t.recipientsSettledTx(ctx, tx, current.TaskID, current.SourceID, current.Locator)
}

func (t *CommitmentSettleTrigger) completeSettledClaimTask(ctx context.Context, settled contacts.ClaimSettlement) (bool, error) {
	var completed bool
	err := t.db.Tx(ctx, func(tx pgx.Tx) error {
		id := ids.From[ids.ActivityKind](*settled.TaskID)
		if _, err := t.tasks.LockTaskForBulkTx(ctx, tx, id); err != nil {
			return err
		}
		ready, err := t.recipientsSettledTx(ctx, tx, settled.TaskID, settled.SourceID, settled.Locator)
		if err != nil || !ready {
			return err
		}
		completed, err = t.tasks.CompleteTaskTx(ctx, tx, id, settled.At)
		return err
	})
	return completed, err
}

func (t *CommitmentSettleTrigger) recipientsSettledTx(ctx context.Context, tx pgx.Tx, taskID *ids.UUID, sourceID ids.UUID, locator string) (bool, error) {
	if taskID != nil {
		ready, err := t.claims.TaskClaimsSettledTx(ctx, tx, *taskID)
		if err != nil || !ready {
			return false, err
		}
	}
	return t.claims.PromiseClaimsSettledTx(ctx, tx, sourceID, locator)
}
