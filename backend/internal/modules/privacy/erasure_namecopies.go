// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// clearSubjectNameCopies destroys the copies of the subject's identifiers that
// live OUTSIDE the subject's own rows — in tables keyed to something else, which
// no column scrub of contact or lead can reach.
//
// Each is a verbatim second copy of what the cascade has just nulled; each is
// keyed to a record that is not the subject, so nothing cascades to it; and each
// has to be given the contact AND the lead twins, because a promoted subject is
// both and clearing one leaves half the copies standing.
func clearSubjectNameCopies(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, leadTwins []ids.UUID) error {
	// The author repair's bookkeeping about this subject.
	//
	// The contact and lead UPDATEs clear `source_author_name` on the records
	// themselves, but the repair keeps a SECOND copy of that free text in
	// source_attribution_repair, plus an unkeyed SHA-256 of it and the operator's
	// batch label, under their own object_type.
	// Both types, and not merely the contact: a promoted subject's lead twin
	// carries its own ledger row keyed `lead`, so clearing one and not the other
	// leaves half the copies standing. One loop rather than two calls, because
	// the pair differ only in which records they name.
	for _, subject := range []struct {
		objectType string
		records    []ids.UUID
	}{
		{contactObject, []ids.UUID{contactID.UUID}},
		{"lead", leadTwins},
	} {
		if err := clearAttributionLedgerNames(ctx, tx, subject.objectType, subject.records); err != nil {
			return fmt.Errorf("privacy: clearing the %s attribution ledger: %w", subject.objectType, err)
		}
	}
	// And the duplicate-pair snapshots naming either end, which hold the name,
	// address and phone number as the detector read them.
	return scrubDedupeEvidence(ctx, tx, []ids.UUID{contactID.UUID}, leadTwins)
}
