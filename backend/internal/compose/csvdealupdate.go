// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A corrected deals file landing on deals an earlier import made.
//
// The dry run and the commit share one plan, so the report says what the commit
// does. A plan holds the fields that changed, a move to another stage, a
// stakeholder to seat, or the reason the row is refused.

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// dealPlan is what one row asks of a deal an import already made.
type dealPlan struct {
	patch   dealPatch
	changed int
	// closedAt is the day the file gives the close, for a move to a closed stage.
	closedAt *time.Time
	// stakeholder is the contact the row names who is not yet on the deal.
	stakeholder *ids.UUID
}

func (p dealPlan) unchanged() bool { return p.changed == 0 && p.stakeholder == nil }

// storedDeal is the part of the stored deal a plan decides on.
type storedDeal struct {
	PipelineID string `json:"pipeline_id"`
	Status     string `json:"status"`
}

// planDealUpdate reads the stored deal and decides what the row changes. A
// refusal comes back as its reason and the row is skipped whole. Half a row
// would leave a deal that matches neither the file nor the CRM.
func (w *csvWriters) planDealUpdate(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (dealPlan, migration.Reason, error) {
	current, err := w.read(ctx, id)
	if err != nil {
		return dealPlan{}, migration.Reason{}, err
	}
	fields := textFields(row.Fields)
	changed, err := changedFields(current, fields, typed)
	if err != nil {
		return dealPlan{}, migration.Reason{}, err
	}
	var stored storedDeal
	if err := json.Unmarshal(current, &stored); err != nil {
		return dealPlan{}, migration.Reason{}, fmt.Errorf("import: reading the stored deal: %w", err)
	}
	plan := dealPlan{patch: dealUpdateFrom(changed, typed), changed: len(changed), closedAt: dealCloseDate(fields, typed)}
	if move := plan.patch.Move; move != nil {
		if pipeline, ok := typed[dealTargetPipeline].(ids.PipelineID); ok && pipeline.String() != stored.PipelineID {
			return plan, migration.Why(migration.ReasonDealPipelineFixed), nil
		}
		if stored.Status != string(deals.DealOpen) {
			return plan, migration.Why(migration.ReasonDealAlreadyClosed), nil
		}
		move.LostReason = importString(fields, dealTargetLostReason)
	}
	if w.staysOpenOnPastDate(fields, plan.patch, stored) {
		return plan, migration.Why(migration.ReasonCloseDatePast, "value", fields[dealTargetCloseDate]), nil
	}
	contact, seat, err := w.stakeholderToSeat(ctx, id, typed)
	if seat {
		plan.stakeholder = &contact
	}
	return plan, migration.Reason{}, err
}

// staysOpenOnPastDate answers whether the row would leave an open deal with a
// close date in the past. The store refuses that for a live edit, and an import
// does not get around it for a deal that stays open.
func (w *csvWriters) staysOpenOnPastDate(fields map[string]string, patch dealPatch, stored storedDeal) bool {
	if patch.Deal.ExpectedClose == nil || !deals.CloseDateIsPast(*patch.Deal.ExpectedClose, w.dealStages.today) {
		return false
	}
	if patch.Move == nil {
		return stored.Status == string(deals.DealOpen)
	}
	choice, problem := w.dealStages.resolve(fields[dealTargetPipeline], fields[dealTargetStage])
	return problem == nil && choice.semantic == crmcontracts.StageSemanticOpen
}

// stakeholderToSeat is the contact the row names when the deal does not hold
// it yet, and whether there is one to add.
func (w *csvWriters) stakeholderToSeat(ctx context.Context, dealID ids.UUID, typed map[string]any) (ids.UUID, bool, error) {
	contact, ok := linkedRecord(typed, dealTargetContactRef)
	if !ok {
		return ids.UUID{}, false, nil
	}
	var seated bool
	if err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		// Not read through the caller's edge scope: an invisible edge is still
		// a seat, and counting it absent would add a second one.
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM relationship r
			                WHERE r.kind = $1 AND r.deal_id = $2 AND r.contact_id = $3
			                  AND r.archived_at IS NULL)`,
			relationshipKindStakeholder, dealID, contact).Scan(&seated)
	}); err != nil {
		return ids.UUID{}, false, fmt.Errorf("import: reading the deal's stakeholders: %w", err)
	}
	return contact, !seated, nil
}

// reconcileDeal brings a deal up to what the file now says: its fields, its
// stage and its stakeholder, in one transaction.
func (w *csvWriters) reconcileDeal(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (migration.EnsureResult, error) {
	plan, refusal, err := w.planDealUpdate(ctx, id, row, typed)
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if refusal.Code != "" {
		return skippedFor(refusal), nil
	}
	if plan.unchanged() {
		return migration.EnsureResult{Unchanged: true}, w.tagExisting(ctx, id, row)
	}
	if !auth.Allows(ctx, w.object, principal.ActionUpdate) {
		return skippedFor(cannotEditWhy(w.object)), nil
	}
	err = w.writeDealUpdate(ctx, id, plan, typed, w.provenanceOf(row.ExternalID))
	if why, refused := w.dealRefusal(err, textFields(row.Fields)); refused {
		return skippedFor(why), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	if err := w.tagExisting(ctx, id, row); err != nil {
		return migration.EnsureResult{}, err
	}
	w.updated++
	return migration.EnsureResult{Disclosure: fmt.Sprintf("row %s: %d field(s) rewritten from the file", row.ExternalID, plan.changed)}, nil
}

// writeDealUpdate lands a plan. The move comes first: closing the deal is what
// lets a past close date stand, so the fields are written against the deal's
// new status.
func (w *csvWriters) writeDealUpdate(ctx context.Context, id ids.UUID, plan dealPlan, typed map[string]any, source string) error {
	ctx = contacts.WithBulkWrite(ctx)
	deal := ids.From[ids.DealKind](id)
	return database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		if plan.patch.Move != nil {
			if _, err := w.deals.ImportAdvanceDealTx(ctx, tx, deal, *plan.patch.Move, plan.closedAt, w.cols.deals); err != nil {
				return err
			}
		}
		if plan.changed > 0 {
			if _, err := w.deals.UpdateDealTx(ctx, tx, deal, plan.patch.Deal, w.cols.deals); err != nil {
				return err
			}
		}
		if plan.stakeholder != nil {
			return w.seatStakeholder(ctx, tx, id, typed, source)
		}
		return nil
	})
}

// predictDealUpdate is reconcilePrediction for a deal: the same plan, not
// written.
func (w *csvWriters) predictDealUpdate(
	ctx context.Context, id ids.UUID, row migration.Row, typed map[string]any,
) (predictedOutcome, migration.Reason, error) {
	plan, refusal, err := w.planDealUpdate(ctx, id, row, typed)
	switch {
	case err != nil:
		return predictCreate, migration.Reason{}, err
	case refusal.Code != "":
		return predictUnwritable, refusal, nil
	case plan.unchanged():
		return predictUnchanged, migration.Reason{}, nil
	case !auth.Allows(ctx, w.object, principal.ActionUpdate):
		return predictUnwritable, cannotEditWhy(w.object), nil
	}
	return predictUpdate, migration.Reason{}, nil
}
