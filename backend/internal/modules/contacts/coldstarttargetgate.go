// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The row-scope gate on an accepted cold-start read-back.
//
// A cold-start proposal is staged against a URL, not against a record: which
// company it writes is only known once the domain resolves, inside the
// apply. So the gate cannot sit at the entry point the way a normal update's
// does — ApplyColdStartProfile's auth.Require answers "may this seat update
// companies at all", which is a different question from "may it write
// THIS one".

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// gateResolvedColdStartTarget refuses an apply that landed on a company the
// approver may not write.
//
// Only when the company already existed. A row this apply just minted has
// no prior owner to consult, and gating it would refuse every genuine cold
// start — the case the whole path exists for.
//
// The read-back may replace a description that no contact authored, so landing
// on an existing record is a write to somebody's record rather than merely a
// fill of empty columns. ApplyDeepReadTx takes the same gate at the same point
// in its own flow.
//
// LIVE, for the reason every staged apply is: the proposal is raised against a
// URL and applied later, and an archive landing in that window is the ordinary
// case rather than a race. Without the filter an accepted read-back writes a
// legal name, an industry and an address onto a company somebody deliberately
// retired, and ships a company.updated event for a record the record's own
// PATCH refuses.
func gateResolvedColdStartTarget(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, created bool) error {
	if created {
		return nil
	}
	return auth.EnsureWritableLive(ctx, tx, entityCompany, companyID.UUID)
}
