// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The bulk change's own row: who asked, for what, how it went per record, and
// which undo reversed it. The records it changed carry its id on their audit
// rows; this row is what GET /v1/bulk/{id} answers and what an undo reads.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bulkOutcome is one changed record as an undo needs it: the version the
// change left it at, and the owner it had before.
type bulkOutcome struct {
	ID          openapi_types.UUID  `json:"id"`
	Version     int64               `json:"version"`
	OwnerBefore *openapi_types.UUID `json:"owner_before,omitempty"`
}

// bulkResult is bulk_operation.result. A skip keeps its reason and code but
// not its message or parameters: those are a rule's own words about a record,
// and this row outlives the request that was shown them.
type bulkResult struct {
	Changed    []bulkOutcome                 `json:"changed"`
	Skipped    []crmcontracts.BulkSkip       `json:"skipped"`
	LeftBehind []crmcontracts.BulkLeftBehind `json:"left_behind,omitempty"`
}

// recordBulkOperation writes the change's own row, and for an undo marks the
// change it reversed as undone — refusing when another undo got there first.
func recordBulkOperation(ctx context.Context, tx pgx.Tx, batchID ids.UUID, change bulkChange, run bulkRun) error {
	actor, err := storekit.Actor(ctx)
	if err != nil {
		return err
	}
	named := map[string]ids.UUID{}
	if change.ownerID != nil {
		named["owner_id"] = *change.ownerID
	}
	params, err := json.Marshal(named)
	if err != nil {
		return fmt.Errorf("record the change's parameters: %w", err)
	}
	result, err := json.Marshal(storedResult(run))
	if err != nil {
		return fmt.Errorf("record the change's result: %w", err)
	}
	var undoOf *ids.UUID
	if change.undo != nil {
		undoOf = &change.undo.batchID
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO bulk_operation (id, record_type, verb, params, requested_by, passport_id, changed_count, skipped_count, result, undo_of)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		batchID, string(change.recordType), string(change.verb), params, actor.ID,
		storekit.UUIDOrNil(actor.PassportID), len(run.changed), len(run.skipped), result, undoOf)
	if storekit.IsUniqueViolation(err) {
		return errBulkAlreadyUndone
	}
	if err != nil {
		return fmt.Errorf("record the bulk change: %w", err)
	}
	if undoOf == nil {
		return nil
	}
	tag, err := tx.Exec(ctx, `UPDATE bulk_operation SET undone_by = $2 WHERE id = $1 AND undone_by IS NULL`, *undoOf, batchID)
	if err != nil {
		return fmt.Errorf("mark the bulk change undone: %w", err)
	}
	if tag.RowsAffected() != 1 {
		return errBulkAlreadyUndone
	}
	return nil
}

func storedResult(run bulkRun) bulkResult {
	out := bulkResult{Changed: run.outcomes, Skipped: make([]crmcontracts.BulkSkip, len(run.skipped)), LeftBehind: run.leftBehind}
	if out.Changed == nil {
		out.Changed = []bulkOutcome{}
	}
	for i, skip := range run.skipped {
		out.Skipped[i] = crmcontracts.BulkSkip{Id: skip.Id, Reason: skip.Reason, Code: skip.Code}
	}
	return out
}

// bulkOperation is one bulk_operation row as its readers need it.
type bulkOperation struct {
	id           ids.UUID
	recordType   crmcontracts.BulkRecordType
	verb         crmcontracts.BulkVerb
	ownerID      *ids.UUID
	requestedBy  string
	changedCount int
	result       bulkResult
	undoOf       *ids.UUID
	undoneBy     *ids.UUID
	createdAt    time.Time
}

// readBulkOperation reads one batch the caller may read: the colleague who asked
// for it, or an administrator. Anyone else is answered ErrNotFound, exactly as
// for a batch that does not exist.
func readBulkOperation(ctx context.Context, tx pgx.Tx, id ids.UUID) (bulkOperation, error) {
	op := bulkOperation{id: id}
	var params, result []byte
	err := tx.QueryRow(ctx,
		`SELECT record_type, verb, params, requested_by, changed_count, result, undo_of, undone_by, created_at
		   FROM bulk_operation WHERE id = $1`, id).
		Scan(&op.recordType, &op.verb, &params, &op.requestedBy, &op.changedCount, &result, &op.undoOf, &op.undoneBy, &op.createdAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return bulkOperation{}, apperrors.ErrNotFound
	}
	if err != nil {
		return bulkOperation{}, fmt.Errorf("read bulk change %s: %w", id, err)
	}
	if !mayReadBatch(ctx, op.requestedBy) {
		return bulkOperation{}, apperrors.ErrNotFound
	}
	var named struct {
		OwnerID *ids.UUID `json:"owner_id"`
	}
	if err := json.Unmarshal(params, &named); err != nil {
		return bulkOperation{}, fmt.Errorf("read bulk change %s parameters: %w", id, err)
	}
	op.ownerID = named.OwnerID
	if err := json.Unmarshal(result, &op.result); err != nil {
		return bulkOperation{}, fmt.Errorf("read bulk change %s result: %w", id, err)
	}
	return op, nil
}

// mayReadBatch admits the principal that asked, the human behind it (who may
// ask an agent to undo what they did on the website), and an administrator.
func mayReadBatch(ctx context.Context, requestedBy string) bool {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return false
	}
	if actor.ID == requestedBy {
		return true
	}
	if human, isHuman := principal.HumanUserID(requestedBy); isHuman && (actor.UserID == human || actor.OnBehalfOf == human) {
		return true
	}
	return auth.RequireAdmin(ctx) == nil
}

// Status answers one bulk change as GET /v1/bulk/{id} reads it.
func (e *bulkEngine) Status(ctx context.Context, id ids.UUID) (crmcontracts.BulkOperation, error) {
	var op bulkOperation
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		op, err = readBulkOperation(ctx, tx, id)
		return err
	})
	if err != nil {
		return crmcontracts.BulkOperation{}, err
	}
	leftBehind := op.result.LeftBehind
	if leftBehind == nil {
		leftBehind = []crmcontracts.BulkLeftBehind{}
	}
	skipped := op.result.Skipped
	if skipped == nil {
		skipped = []crmcontracts.BulkSkip{}
	}
	return crmcontracts.BulkOperation{
		BatchId: openapi_types.UUID(op.id), RecordType: op.recordType, Verb: op.verb,
		OwnerId: wireOwner(op.ownerID), Changed: op.changedCount, Skipped: skipped, LeftBehind: leftBehind,
		UndoOf: wireOwner(op.undoOf), UndoneBy: wireOwner(op.undoneBy), CreatedAt: op.createdAt,
	}, nil
}
