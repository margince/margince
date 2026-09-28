// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The one bulk-change engine behind POST /v1/bulk/preview, POST /v1/bulk/execute
// and the bulk_update_records tool.
//
// A bulk change is the single-record write, N times, in one transaction. Rows
// are locked in id order so two changes over overlapping selections queue
// instead of deadlocking; each row runs inside its own savepoint, so a row a
// rule refuses is left alone and reported while the others go ahead. Every
// audit row the change writes carries its batch id (storekit.WithBatch).
//
// A preview runs exactly the same rows inside a savepoint it then rolls back.
// That is what makes its exclusions the execution's exclusions: nothing decides
// them but the writes themselves.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bulkToolName is the agent verb both doors admit a bulk change as.
const bulkToolName = "bulk_update_records"

// bulkMaxItems mirrors the contract's bound on a selection.
const bulkMaxItems = 500

// bulkSampleSize is how many before/after rows a preview shows the user.
const bulkSampleSize = 3

// bulkChange is one requested change, whichever door it came through.
type bulkChange struct {
	recordType   crmcontracts.BulkRecordType
	verb         crmcontracts.BulkVerb
	items        []crmcontracts.BulkItem
	ownerID      *ids.UUID
	confirmToken string
	// previewed is the set the spent confirmation's preview listed as
	// affected; nil when the change presented none. A record outside it is
	// left alone, so a change never reaches further than the user was shown.
	previewed map[openapi_types.UUID]bool
}

// bulkEngine runs bulk changes over the three record types.
type bulkEngine struct {
	db      *database.DB
	targets map[crmcontracts.BulkRecordType]bulkTarget
	gate    *auth.Gate
	now     func() time.Time
}

// bulkRun is what one pass over the rows produced.
type bulkRun struct {
	changed []crmcontracts.BulkSampleRow
	skipped []crmcontracts.BulkSkip
}

// Preview answers what change would do, and writes nothing but the
// confirmation that holds an execution to what it showed.
func (e *bulkEngine) Preview(ctx context.Context, change bulkChange) (crmcontracts.BulkChangePreview, error) {
	records, err := e.admit(ctx, change)
	if err != nil {
		return crmcontracts.BulkChangePreview{}, err
	}
	out := crmcontracts.BulkChangePreview{
		RecordType: change.recordType, Verb: change.verb,
		RequiresConfirmation: len(change.items) > bulkConfirmAbove,
	}
	err = e.transact(ctx, func(tx pgx.Tx) error {
		run, err := e.rehearse(ctx, tx, records.target, change)
		if err != nil {
			return err
		}
		out.Count, out.Affected, out.Excluded = len(run.changed), affectedIDs(run.changed), run.skipped
		out.Sample = run.changed[:min(len(run.changed), bulkSampleSize)]
		if out.Count == 0 {
			return nil
		}
		token, expires, err := mintBulkConfirmation(ctx, tx, change, out.Affected, e.now())
		out.ConfirmToken, out.ExpiresAt = &token, &expires
		return err
	})
	// A preview changes no record, so it charges none against a write budget.
	agentvolume.NoteEffects(ctx, 0)
	return out, err
}

// Execute applies change and answers what changed and what was left alone.
func (e *bulkEngine) Execute(ctx context.Context, change bulkChange) (crmcontracts.BulkChangeResult, error) {
	records, err := e.admit(ctx, change)
	if err != nil {
		return crmcontracts.BulkChangeResult{}, err
	}
	if len(change.items) > bulkConfirmAbove && change.confirmToken == "" {
		return crmcontracts.BulkChangeResult{}, errConfirmTokenRequired
	}
	batchID := ids.NewV7()
	ctx = storekit.WithBatch(ctx, batchID)
	var run bulkRun
	var reserved *auth.WriteReservation
	err = e.transact(ctx, func(tx pgx.Tx) error {
		// An attempt a deadlock aborted committed nothing, so what it reserved
		// goes back before this one reserves again.
		reserved.Refund(ctx)
		var err error
		if run, reserved, err = e.applyAndReserve(ctx, tx, records.target, change); err != nil {
			return err
		}
		return recordBulkOperation(ctx, tx, batchID, change, run)
	})
	if err != nil {
		reserved.Refund(ctx)
		return crmcontracts.BulkChangeResult{}, err
	}
	// The reservation already charged every changed record, so the door that
	// charges a call's effects afterwards charges nothing more.
	agentvolume.NoteEffects(ctx, 0)
	return crmcontracts.BulkChangeResult{BatchId: openapi_types.UUID(batchID), Changed: len(run.changed), Skipped: run.skipped}, nil
}

// applyAndReserve spends the confirmation, changes the rows it covers, and
// reserves the changed count against an agent's write budget before the
// commit, while refusing still costs nothing.
func (e *bulkEngine) applyAndReserve(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange,
) (bulkRun, *auth.WriteReservation, error) {
	var previewed map[openapi_types.UUID]bool
	if change.confirmToken != "" {
		affected, err := spendBulkConfirmation(ctx, tx, change, change.confirmToken, e.now())
		if err != nil {
			return bulkRun{}, nil, err
		}
		previewed = affected
	}
	change.previewed = previewed
	run, err := e.apply(ctx, tx, target, change)
	if err != nil {
		return bulkRun{}, nil, err
	}
	reserved, err := e.gate.ReserveRecordWrites(ctx, bulkToolName, len(run.changed))
	return run, reserved, err
}

// admit refuses a malformed change and a caller who may not make this change
// to ANY record of the type, before a transaction is opened, and answers the
// record type's share of the change.
func (e *bulkEngine) admit(ctx context.Context, change bulkChange) (bulkRecords, error) {
	target, ok := e.targets[change.recordType]
	if !ok {
		return bulkRecords{}, httperr.Validation("record_type", "unknown_record_type",
			fmt.Sprintf("record_type %q is none of contact, company or deal", change.recordType))
	}
	if err := validateBulkChange(change); err != nil {
		return bulkRecords{}, err
	}
	action := principal.ActionUpdate
	if change.verb == crmcontracts.BulkVerbArchive {
		action = principal.ActionDelete
	}
	if err := auth.Require(ctx, string(change.recordType), action); err != nil {
		return bulkRecords{}, err
	}
	return bulkRecords{target: target}, nil
}

// bulkRecords is the admitted record type's share of a change.
type bulkRecords struct{ target bulkTarget }

// transact runs one attempt, and a second when Postgres broke a lock cycle by
// aborting the first. Rows are locked in id order, so a cycle needs a writer
// outside this engine, and that writer has usually committed by the retry.
func (e *bulkEngine) transact(ctx context.Context, fn func(pgx.Tx) error) error {
	err := e.db.Tx(ctx, fn)
	if storekit.IsDeadlock(err) {
		err = e.db.Tx(ctx, fn)
	}
	return err
}

// rehearse runs apply inside a savepoint it always rolls back.
func (e *bulkEngine) rehearse(ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange) (bulkRun, error) {
	rehearsal, err := tx.Begin(ctx)
	if err != nil {
		return bulkRun{}, fmt.Errorf("open the preview's savepoint: %w", err)
	}
	run, err := e.apply(ctx, rehearsal, target, change)
	if rollbackErr := rehearsal.Rollback(ctx); rollbackErr != nil && err == nil {
		err = fmt.Errorf("roll the preview back: %w", rollbackErr)
	}
	return run, err
}

// apply changes every row in id order, each in its own savepoint.
func (e *bulkEngine) apply(ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange) (bulkRun, error) {
	if change.ownerID != nil {
		if err := auth.EnsureAssignee(ctx, tx, *change.ownerID); err != nil {
			return bulkRun{}, err
		}
	}
	items := slices.Clone(change.items)
	slices.SortFunc(items, func(a, b crmcontracts.BulkItem) int { return strings.Compare(a.Id.String(), b.Id.String()) })
	run := bulkRun{changed: []crmcontracts.BulkSampleRow{}, skipped: []crmcontracts.BulkSkip{}}
	for _, item := range items {
		row, skip, err := applyOneInSavepoint(ctx, tx, target, change, item)
		switch {
		case err != nil:
			return bulkRun{}, err
		case skip != nil:
			run.skipped = append(run.skipped, *skip)
		default:
			run.changed = append(run.changed, row)
		}
	}
	return run, nil
}

// applyOneInSavepoint changes one row, or rolls its savepoint back and says why
// the row was left alone.
func applyOneInSavepoint(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (crmcontracts.BulkSampleRow, *crmcontracts.BulkSkip, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return crmcontracts.BulkSampleRow{}, nil, fmt.Errorf("open a savepoint for %s: %w", item.Id, err)
	}
	row, skip, err := applyOne(ctx, savepoint, target, change, item)
	if err == nil && skip.Reason == "" {
		if err := savepoint.Commit(ctx); err != nil {
			return crmcontracts.BulkSampleRow{}, nil, fmt.Errorf("release the savepoint for %s: %w", item.Id, err)
		}
		return row, nil, nil
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return crmcontracts.BulkSampleRow{}, nil, fmt.Errorf("roll back the savepoint for %s: %w", item.Id, rollbackErr)
	}
	if err != nil {
		return crmcontracts.BulkSampleRow{}, nil, err
	}
	skip.Id = item.Id
	return crmcontracts.BulkSampleRow{}, &skip, nil
}

// skipped is a row left alone for reason, with nothing further to say.
func skipped(reason crmcontracts.BulkSkipReason) crmcontracts.BulkSkip {
	return crmcontracts.BulkSkip{Reason: reason}
}

// applyOne is one row's change. A row a rule refuses answers why it was left
// alone; an error nobody classified aborts the whole change.
func applyOne(
	ctx context.Context, tx pgx.Tx, target bulkTarget, change bulkChange, item crmcontracts.BulkItem,
) (crmcontracts.BulkSampleRow, crmcontracts.BulkSkip, error) {
	if change.previewed != nil && !change.previewed[item.Id] {
		return crmcontracts.BulkSampleRow{}, skipped(crmcontracts.BulkSkipReasonNotPreviewed), nil
	}
	id := ids.UUID(item.Id)
	row, err := target.lock(ctx, tx, id)
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return crmcontracts.BulkSampleRow{}, skip, classifyErr
	}
	if row.version != item.Version {
		return crmcontracts.BulkSampleRow{}, skipped(crmcontracts.BulkSkipReasonChangedSincePreview), nil
	}
	sample := crmcontracts.BulkSampleRow{
		Id: item.Id, Label: row.label,
		Before: crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID)},
		After:  crmcontracts.BulkRecordState{OwnerId: wireOwner(row.ownerID)},
	}
	switch change.verb {
	case crmcontracts.BulkVerbReassignOwner:
		if row.ownerID != nil && *row.ownerID == *change.ownerID {
			return crmcontracts.BulkSampleRow{}, skipped(crmcontracts.BulkSkipReasonNoChange), nil
		}
		sample.After.OwnerId = wireOwner(change.ownerID)
		err = target.reassign(ctx, tx, id, ids.From[ids.UserKind](*change.ownerID), item.Version)
	case crmcontracts.BulkVerbArchive:
		sample.After.Archived = true
		err = target.archive(ctx, tx, id, item.Version)
	}
	if err != nil {
		skip, classifyErr := bulkSkipFor(err)
		return crmcontracts.BulkSampleRow{}, skip, classifyErr
	}
	return sample, crmcontracts.BulkSkip{}, nil
}

// bulkSkipFor turns a row's refusal into why it is left alone. A refusal a
// single-record rule answers carries that rule's own code, so a client can say
// it in the reader's language; the English message stays beside it for a code
// the client does not know. An error that is no refusal — a lost connection, a
// bug — is answered as itself, and aborts the change rather than being
// reported as one row's fault.
func bulkSkipFor(err error) (crmcontracts.BulkSkip, error) {
	var anchor *contacts.AnchorProtectedError
	switch {
	case errors.Is(err, apperrors.ErrNotFound):
		return skipped(crmcontracts.BulkSkipReasonNotFound), nil
	case errors.Is(err, apperrors.ErrPermissionDenied):
		return skipped(crmcontracts.BulkSkipReasonNotWritable), nil
	case errors.Is(err, apperrors.ErrVersionSkew):
		return skipped(crmcontracts.BulkSkipReasonChangedSincePreview), nil
	case errors.As(err, &anchor):
		return skipped(crmcontracts.BulkSkipReasonAnchorCompany), nil
	}
	fault, classified := httperr.Classify(err)
	if !classified || fault.Transient() || fault.Status < http.StatusBadRequest || fault.Status >= http.StatusInternalServerError {
		return crmcontracts.BulkSkip{}, err
	}
	code := fault.Code
	if len(fault.Fields) > 0 {
		code = fault.Fields[0].Code
	}
	skip := crmcontracts.BulkSkip{Reason: crmcontracts.BulkSkipReasonRefused, Code: &code, Message: &fault.Detail}
	if len(fault.Details) > 0 {
		skip.Params = &fault.Details
	}
	return skip, nil
}

// recordBulkOperation writes the change's own row: who asked, for what, and how
// it went. The records it changed carry its id on their audit rows.
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
	_, err = tx.Exec(ctx,
		`INSERT INTO bulk_operation (id, record_type, verb, params, requested_by, passport_id, changed_count, skipped_count)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)`,
		batchID, string(change.recordType), string(change.verb), params, actor.ID,
		storekit.UUIDOrNil(actor.PassportID), len(run.changed), len(run.skipped))
	if err != nil {
		return fmt.Errorf("record the bulk change: %w", err)
	}
	return nil
}

func affectedIDs(changed []crmcontracts.BulkSampleRow) []openapi_types.UUID {
	out := make([]openapi_types.UUID, len(changed))
	for i, row := range changed {
		out[i] = row.Id
	}
	return out
}

func wireOwner(id *ids.UUID) *openapi_types.UUID {
	if id == nil {
		return nil
	}
	wire := openapi_types.UUID(*id)
	return &wire
}

// validateBulkChange refuses what the contract refuses, for the tool door
// whose arguments no generated decoder has checked, and one thing it cannot
// say: a record named twice.
func validateBulkChange(change bulkChange) error {
	switch change.verb {
	case crmcontracts.BulkVerbReassignOwner:
		if change.ownerID == nil {
			return httperr.Validation("owner_id", "required", "reassign_owner needs owner_id, the colleague to hand the records to")
		}
	case crmcontracts.BulkVerbArchive:
		if change.ownerID != nil {
			return httperr.Validation("owner_id", "not_allowed", "archive takes no owner_id")
		}
	default:
		return httperr.Validation("verb", "unknown_verb", fmt.Sprintf("verb %q is neither reassign_owner nor archive", change.verb))
	}
	if len(change.items) == 0 || len(change.items) > bulkMaxItems {
		return httperr.Validation("items", "out_of_range",
			fmt.Sprintf("items names between 1 and %d records; this change names %d", bulkMaxItems, len(change.items)))
	}
	seen := make(map[openapi_types.UUID]bool, len(change.items))
	for _, item := range change.items {
		if seen[item.Id] {
			return httperr.Validation("items", "duplicate_id", fmt.Sprintf("record %s is named more than once", item.Id))
		}
		seen[item.Id] = true
	}
	return nil
}
