// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Bringing an archived deal back. The flow and its refusals are
// storekit/unarchive.go's; this is the deal's share of the statements.
//
// The relationship statement repeats the contacts module's: a module never imports
// a sibling, and the table-ownership gate sees only a literal in the writer.

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var dealUnarchive = storekit.UnarchiveShape{
	Table: dealTable,
	// A deal is never merged, so it answers false for the merged column.
	Lock: `SELECT name, archived_at, false, version FROM deal WHERE id = $1 FOR UPDATE`,
	Children: []storekit.ChildRestore{{Table: "relationship", Statement: `UPDATE relationship r SET archived_at = NULL
		WHERE r.id = $1 AND r.archived_at = $2
		  AND NOT EXISTS (SELECT 1 FROM contact c WHERE c.id IN (r.contact_id, r.counterparty_contact_id) AND c.archived_at IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM company c WHERE c.id IN (r.company_id, r.counterparty_company_id) AND c.archived_at IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM deal d WHERE d.id = r.deal_id AND d.archived_at IS NOT NULL)
		  AND NOT EXISTS (SELECT 1 FROM project p WHERE p.id = r.project_id AND p.archived_at IS NOT NULL)`}},
	Restored: crmcontracts.PublicEventDealRestored{},
}

// RestoreDealTx brings an archived deal back on the caller's transaction,
// conditioned on ifVersion, behind the gates the archive asks.
func (s *Store) RestoreDealTx(
	ctx context.Context, tx pgx.Tx, id ids.DealID, ifVersion *int64, with storekit.RestoreWith,
) (storekit.RestoreReport, error) {
	if err := auth.Require(ctx, "deal", principal.ActionDelete); err != nil {
		return storekit.RestoreReport{}, err
	}
	// Not the live twin: the row is archived by definition, and bringing it
	// back is the write that reaches it on purpose.
	if err := auth.EnsureWritable(ctx, tx, dealTable, id.UUID); err != nil {
		return storekit.RestoreReport{}, err
	}
	return storekit.Unarchive(ctx, tx, dealUnarchive, id.UUID, ifVersion, with)
}
