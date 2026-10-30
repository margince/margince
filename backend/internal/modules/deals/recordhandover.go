// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RecordDealsHandOver is what handing on a record's open deals did: how many
// moved, and the names of those that stayed with their owner.
type RecordDealsHandOver struct {
	Moved int
	Kept  []string
}

// heldOnRecordSQL selects the open deals owner holds on one contact or company
// and appends their arguments. A contact's deals are those it is a live
// stakeholder of, read under the edge grant; a caller without it finds none.
func heldOnRecordSQL(ctx context.Context, recordType string, recordID ids.UUID, owner ids.UserID, args *[]any) (string, error) {
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	held := fmt.Sprintf(`%s AND d.owner_id = $%d`, openDealSQL, arg(owner))
	switch recordType {
	case "company":
		return fmt.Sprintf(`d.company_id = $%d AND %s`, arg(recordID), held), nil
	case "contact":
		edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "false", nil
		}
		if err != nil {
			return "", err
		}
		if edgeScope == "" {
			edgeScope = predicateAlways
		}
		return fmt.Sprintf(`EXISTS (SELECT 1 FROM relationship r
			 WHERE r.deal_id = d.id AND r.kind = 'deal_stakeholder' AND r.contact_id = $%d
			   AND r.archived_at IS NULL AND (%s)) AND %s`, arg(recordID), edgeScope, held), nil
	}
	return "", fmt.Errorf("deals: no deals hang off a %q", recordType)
}

// CountRecordDealsHeldByTx counts the open deals owner holds on one contact or
// company: the deals HandOverRecordDealsTx would try to move.
func (s *Store) CountRecordDealsHeldByTx(ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, owner ids.UserID) (int, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return 0, err
	}
	var args []any
	where, err := heldOnRecordSQL(ctx, recordType, recordID, owner, &args)
	if err != nil {
		return 0, err
	}
	var held int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM deal d WHERE `+where, args...).Scan(&held); err != nil {
		return 0, fmt.Errorf("deals: counting a record's open deals: %w", err)
	}
	return held, nil
}

// HandOverRecordDealsTx gives the open deals from owns on one contact or
// company to to. Each goes through ReassignDealTx, so each gets its own audit
// row, event and hand-on of its pending proposals. A deal whose own write refuses
// the move stays with from and is named in Kept. A caller without the update
// grant moves none, so the record itself still moves. A closed deal, an
// archived one and one somebody else owns are not touched.
func (s *Store) HandOverRecordDealsTx(
	ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, from, to ids.UserID,
) (RecordDealsHandOver, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return RecordDealsHandOver{}, nil
		}
		return RecordDealsHandOver{}, err
	}
	var args []any
	where, err := heldOnRecordSQL(ctx, recordType, recordID, from, &args)
	if err != nil {
		return RecordDealsHandOver{}, err
	}
	rows, err := tx.Query(ctx, `SELECT d.id, d.name, d.version FROM deal d WHERE `+where+`
		 ORDER BY d.id FOR UPDATE OF d`, args...)
	if err != nil {
		return RecordDealsHandOver{}, fmt.Errorf("deals: reading a record's open deals to hand on: %w", err)
	}
	type candidate struct {
		ID      ids.DealID
		Name    string
		Version int64
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[candidate])
	if err != nil {
		return RecordDealsHandOver{}, fmt.Errorf("deals: reading a record's open deals to hand on: %w", err)
	}
	var out RecordDealsHandOver
	for _, c := range candidates {
		moved, err := s.handOverDealInSavepoint(ctx, tx, c.ID, to, c.Version)
		if err != nil {
			return RecordDealsHandOver{}, err
		}
		if moved {
			out.Moved++
		} else {
			out.Kept = append(out.Kept, c.Name)
		}
	}
	return out, nil
}

// handOverDealInSavepoint moves one deal, and rolls back only that deal when
// its own write refuses. An error that is no refusal fails the whole move.
func (s *Store) handOverDealInSavepoint(ctx context.Context, tx pgx.Tx, id ids.DealID, to ids.UserID, version int64) (bool, error) {
	savepoint, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("deals: opening a savepoint for deal %s: %w", id, err)
	}
	err = s.ReassignDealTx(ctx, savepoint, id, to, &version)
	if err == nil {
		return true, savepoint.Commit(ctx)
	}
	if rollbackErr := savepoint.Rollback(ctx); rollbackErr != nil {
		return false, fmt.Errorf("deals: rolling back the savepoint for deal %s: %w", id, rollbackErr)
	}
	if refusedHandOn(err) {
		return false, nil
	}
	return false, err
}

// refusedHandOn reports whether a deal's own write turned the move down, as
// opposed to failing.
func refusedHandOn(err error) bool {
	if _, ok := errors.AsType[apperrors.FieldFault](err); ok {
		return true
	}
	return errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrConflict) || errors.Is(err, apperrors.ErrVersionSkew)
}
