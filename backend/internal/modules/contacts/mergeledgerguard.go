// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// refuseUnlinkedLedgerRows refuses to merge away a company that holds an
// invoice or payment mirrored before rows named their customer link. The
// finance link checks bind every rewrite of such a row, and the merge has no
// link to name, so the move would fail halfway.
func refuseUnlinkedLedgerRows(ctx context.Context, tx pgx.Tx, source ids.CompanyID) error {
	var unlinked bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
		SELECT 1 FROM finance_invoice WHERE company_id = @source AND customer_link_id IS NULL
		UNION ALL
		SELECT 1 FROM finance_payment WHERE company_id = @source AND customer_link_id IS NULL)`,
		pgx.StrictNamedArgs{mergeSourceKey: source.UUID}).Scan(&unlinked)
	if err != nil {
		return fmt.Errorf("contacts: checking the ledger rows before merging companies: %w", err)
	}
	if unlinked {
		return &UnlinkedLedgerRowsError{}
	}
	return nil
}

// UnlinkedLedgerRowsError refuses the merge without saying which records stay
// behind: a reader without finance access must not learn that invoices exist.
type UnlinkedLedgerRowsError struct{}

func (*UnlinkedLedgerRowsError) Error() string {
	return "Some records on this company can't move to the other company. " +
		"Keep this company as the one that stays, or link its accounting customer first."
}

// Is makes the refusal a conflict (409).
func (*UnlinkedLedgerRowsError) Is(target error) bool { return target == apperrors.ErrConflict }

// MessageFault names the condition; no field of the request can fix it.
func (e *UnlinkedLedgerRowsError) MessageFault() (code, message string) {
	return "company_merge_records_cannot_move", e.Error()
}
