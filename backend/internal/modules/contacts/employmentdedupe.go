// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// employerTwinSource names the queue rows an employment edge files.
const employerTwinSource = "employment_edge_recheck"

// fileEmployerTwins re-scores a contact and files its same-employer twins. The
// create's own duplicate check ran before the contact had an employer, so the
// shared employer counts only after the edge is written.
//
// A company merge moves edges without calling it.
func fileEmployerTwins(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, by string) error {
	return recheckContactForDuplicates(ctx, tx, contactID, employerTwinSource, by)
}
