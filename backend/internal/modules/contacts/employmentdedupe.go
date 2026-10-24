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

// fileEmployerTwins re-scores a contact once it has a current primary employer
// and files the same-employer twins that score finds. A contact created from a
// meeting attendee's name has no employer when the create's duplicate check
// runs, so the shared employer only counts as evidence when this runs after the
// edge is written.
//
// The create, update, archive, capture, domain-triage and provider-claim
// writers call it in their own transaction, so the edge and the pair commit
// together. A company merge moves edges without calling it.
func fileEmployerTwins(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, by string) error {
	return recheckContactForDuplicates(ctx, tx, contactID, employerTwinSource, by)
}
