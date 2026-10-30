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

// RecordDealsHandOver is what handing on a record's open deals did, or would
// do: how many moved, and how many stayed with their owner.
type RecordDealsHandOver struct {
	Moved int
	Kept  int
}

// heldOnRecordSQL selects the open deals owner holds on one contact or company,
// with whether each goes with the record. A company takes all its deals. A
// contact takes a deal only as its sole live stakeholder on a deal with no
// company. Any other deal is somebody else's business too and stays. Only the
// contact's own stakeholder rows are read under the edge grant; the check for
// others decides and returns nothing.
func heldOnRecordSQL(ctx context.Context, recordType string, recordID ids.UUID, owner ids.UserID, args *[]any) (where, moves string, err error) {
	arg := func(v any) int { *args = append(*args, v); return len(*args) }
	held := fmt.Sprintf(`%s AND d.owner_id = $%d`, openDealSQL, arg(owner))
	switch recordType {
	case linkEntityCompany:
		return fmt.Sprintf(`d.company_id = $%d AND %s`, arg(recordID), held), predicateAlways, nil
	case "contact":
		edgeScope, err := auth.EdgeReadScope(ctx, "r", arg)
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return "false", "false", nil
		}
		if err != nil {
			return "", "", err
		}
		if edgeScope == "" {
			edgeScope = predicateAlways
		}
		contact := arg(recordID)
		where = fmt.Sprintf(`EXISTS (SELECT 1 FROM relationship r
			 WHERE r.deal_id = d.id AND r.kind = 'deal_stakeholder' AND r.contact_id = $%d
			   AND r.archived_at IS NULL AND (%s)) AND %s`, contact, edgeScope, held)
		moves = fmt.Sprintf(`d.company_id IS NULL AND NOT EXISTS (SELECT 1 FROM relationship o
			 WHERE o.deal_id = d.id AND o.kind = 'deal_stakeholder' AND o.archived_at IS NULL
			   AND o.contact_id IS DISTINCT FROM $%d)`, contact)
		return where, moves, nil
	}
	return "", "", fmt.Errorf("deals: no deals hang off a %q", recordType)
}

// recordDeal is one open deal the owner holds on the record.
type recordDeal struct {
	ID      ids.DealID
	Version int64
	Moves   bool
}

// recordDealsTx reads the open deals owner holds on the record, locked when
// lock is set. The count and the move both read through it, so the card says
// what the move does.
func recordDealsTx(ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, owner ids.UserID, lock bool) ([]recordDeal, error) {
	var args []any
	where, moves, err := heldOnRecordSQL(ctx, recordType, recordID, owner, &args)
	if err != nil {
		return nil, err
	}
	statement := `SELECT d.id, d.version, (` + moves + `) FROM deal d WHERE ` + where + ` ORDER BY d.id`
	if lock {
		statement += ` FOR UPDATE OF d`
	}
	rows, err := tx.Query(ctx, statement, args...)
	if err != nil {
		return nil, fmt.Errorf("deals: reading a record's open deals: %w", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowToStructByPos[recordDeal])
	if err != nil {
		return nil, fmt.Errorf("deals: reading a record's open deals: %w", err)
	}
	return found, nil
}

// CountRecordDealsHeldByTx answers what HandOverRecordDealsTx would do with
// the open deals owner holds on one contact or company.
func (s *Store) CountRecordDealsHeldByTx(ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, owner ids.UserID) (RecordDealsHandOver, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return RecordDealsHandOver{}, err
	}
	held, err := recordDealsTx(ctx, tx, recordType, recordID, owner, false)
	if err != nil {
		return RecordDealsHandOver{}, err
	}
	var out RecordDealsHandOver
	for _, deal := range held {
		if deal.Moves {
			out.Moved++
		} else {
			out.Kept++
		}
	}
	return out, nil
}

// HandOverRecordDealsTx gives the open deals from owns on one contact or
// company to to, by the rule heldOnRecordSQL states. Each goes through
// ReassignDealTx, so each gets its own audit row, event and hand-on of its
// pending proposals. A deal the rule keeps, or whose own write refuses the
// move, stays with from and is counted in Kept. A caller without the update
// grant moves none, so the record itself still moves.
func (s *Store) HandOverRecordDealsTx(
	ctx context.Context, tx pgx.Tx, recordType string, recordID ids.UUID, from, to ids.UserID,
) (RecordDealsHandOver, error) {
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return RecordDealsHandOver{}, nil
		}
		return RecordDealsHandOver{}, err
	}
	held, err := recordDealsTx(ctx, tx, recordType, recordID, from, true)
	if err != nil {
		return RecordDealsHandOver{}, err
	}
	var out RecordDealsHandOver
	for _, deal := range held {
		moved := false
		if deal.Moves {
			if moved, err = s.handOverDealInSavepoint(ctx, tx, deal.ID, to, deal.Version); err != nil {
				return RecordDealsHandOver{}, err
			}
		}
		if moved {
			out.Moved++
		} else {
			out.Kept++
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
