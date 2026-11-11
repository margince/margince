// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func refuseCrossCompanyContracts(ctx context.Context, tx pgx.Tx, source, target ids.CompanyID) error {
	var mismatch bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (
 SELECT 1 FROM deal d JOIN contract c ON c.deal_id = d.id
 WHERE d.company_id = ANY(@companies) AND c.company_id <> ALL(@companies))`, pgx.StrictNamedArgs{"companies": []ids.UUID{source.UUID, target.UUID}}).Scan(&mismatch)
	if err != nil {
		return fmt.Errorf("contacts: checking contracts before merging companies: %w", err)
	}
	if mismatch {
		return &CrossCompanyContractsError{}
	}
	return nil
}

// CrossCompanyContractsError refuses merges that would retain a contract/deal company mismatch.
type CrossCompanyContractsError struct{}

func (*CrossCompanyContractsError) Error() string {
	return "A deal has a contract belonging to another company. Ask an administrator to correct the contract links before merging."
}

// FieldFault keeps the refusal on the merge's survivor field.
func (e *CrossCompanyContractsError) FieldFault() (field, code, message string) {
	return targetIDField, "company_merge_contract_mismatch", e.Error()
}
