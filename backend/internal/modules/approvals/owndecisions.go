// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// scanOwnDecisions is the probe-free walk for a read the SQL already scopes
// to the caller's own decided rows — see the FailedForDecider branch in List
// for why re-probing decidability would hide exactly the rows it exists for.
func scanOwnDecisions(ctx context.Context, tx pgx.Tx, in ListInput, start *keysetStart) ([]row, storekit.Page, error) {
	q, args := approvalPageQuery(in, start)
	batch, err := collect(ctx, tx, q, args)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return capPage(batch, in.Limit, nil)
}
