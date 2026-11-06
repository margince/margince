// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

type backfillStart struct {
	resume            bool
	internalMailToken string
}

// StartBackfillWithInternalHistory needs both prior administrator approval and the owner's explicit preview.
func (r *Registry) StartBackfillWithInternalHistory(ctx context.Context, provider string, owner ids.UserID, months int, estimate connector.BackfillEstimate, token string, enqueue EnqueueBackfill) (BackfillRun, error) {
	if token == "" {
		return BackfillRun{}, apperrors.ErrInvalidArgument
	}
	return r.openBackfill(ctx, provider, owner, months, estimate, enqueue, backfillStart{internalMailToken: token})
}

func insertBackfillRunTx(ctx context.Context, tx pgx.Tx, run *BackfillRun, estimate connector.BackfillEstimate, history *internalMailHistoryAuthorization) error {
	var encoded json.RawMessage
	if history != nil {
		var err error
		encoded, err = json.Marshal(history)
		if err != nil {
			return err
		}
	}
	var total *int
	if estimate.Messages != 0 {
		total = new(estimate.Messages)
	}
	createdAt, err := nextBackfillAccountTimeTx(ctx, tx, run.ConnectionID)
	if err != nil {
		return err
	}
	args := []any{run.ConnectionID, run.WindowMonths, run.AfterDate, total, estimate.Floor, run.Status, encoded, createdAt}
	err = tx.QueryRow(ctx, `INSERT INTO capture_backfill
 (connection_id, window_months, after_date, total_estimate, total_estimate_is_floor, status, internal_mail_authorization, created_at, started_at)
 VALUES (`+storekit.Placeholders(args)+`, now()) RETURNING id`, args...).Scan(&run.ID)
	if storekit.IsUniqueViolation(err) {
		return ErrBackfillRunning
	}
	return err
}

func auditOpenedBackfillTx(ctx context.Context, tx pgx.Tx, run BackfillRun, history *internalMailHistoryAuthorization, widest *int) error {
	before := map[string]any{auditBackfillWindowMonths: widest}
	after := map[string]any{auditBackfillWindowMonths: run.WindowMonths}
	if history == nil {
		return auditLifecycle(ctx, tx, "update", captureConnectionObject, run.ConnectionID, before, after)
	}
	after["internal_mail_backfill_id"], after["received_after"], after["received_before"] = run.ID, history.After, history.Before
	auditID, err := storekit.Audit(ctx, tx, "update", captureConnectionObject, run.ConnectionID, before, after)
	if err != nil {
		return err
	}
	return storekit.EmitEvent(ctx, tx, auditID, run.ConnectionID, crmcontracts.InternalEventInternalMailHistoryCreated{
		BackfillId: openapi_types.UUID(run.ID), ReceivedAfter: history.After, ReceivedBefore: history.Before,
	})
}
