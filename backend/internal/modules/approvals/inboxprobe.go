// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// inboxProbe is the eligibility every inbox read asks of a batch, by position.
// A row is eligible when it is in the requested scope and p could decide it now.
func inboxProbe(ctx context.Context, tx pgx.Tx, p principal.Principal, in ListInput) batchProbe {
	return func(batch []row) ([]bool, error) {
		return askWhere(batch, func(a row) bool {
			return requestedInScope(a, p.UserID, in) && (in.actionableAt == nil || a.PendingAt(*in.actionableAt))
		},
			func(rows []row) ([]bool, error) { return decidableRows(ctx, tx, p, rows) })
	}
}

// batchProbe answers a question about each row of a batch, by position.
type batchProbe func([]row) ([]bool, error)

// askWhere asks probe about the rows keep admits and refuses the rest, so a
// row an in-memory test refuses never reaches a statement.
func askWhere(batch []row, keep func(row) bool, probe batchProbe) ([]bool, error) {
	var kept []row
	var at []int
	for i, a := range batch {
		if keep(a) {
			kept, at = append(kept, a), append(at, i)
		}
	}
	out := make([]bool, len(batch))
	if len(kept) == 0 {
		return out, nil
	}
	answers, err := probe(kept)
	if err != nil {
		return nil, err
	}
	for j, i := range at {
		out[i] = answers[j]
	}
	return out, nil
}
