// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Writing an accepted correction onto the contact record.
//
// consent owns the proposal and the decision; contacts owns the record and what
// may be written to it. Neither imports the other, so the edge is injected here
// like every other cross-module edge.
//
// THE ORDINARY UPDATE PATH, deliberately. An accepted correction reaches the
// record through the same call any other edit uses, so the same row-scope probe,
// the same audit row and the same event govern it. A writer of its own would be
// a second way to change a contact, answerable to none of that — and the
// subject's own correction is the last edit that should travel on a quieter
// road than a rep's typo fix.

import (
	"context"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type correctionApplier struct {
	store *contacts.Store
}

// ApplyCorrection writes one accepted field.
//
// TWO OF THE FOUR correctable fields are writable here and two are not. Name
// and title are columns on the contact row and UpdateContact takes them. Email
// and phone live on satellite tables with their own primary-address rules,
// verification state and uniqueness, owned by writers that are not one call —
// so this answers ErrUnsupportedField rather than reaching around them, and the
// reviewer is told the record did not move.
//
// The alternative was an accept that silently changed nothing, which is the one
// outcome nobody could act on: the subject would be told their correction was
// taken and the record would still disagree.
func (a correctionApplier) ApplyCorrection(
	ctx context.Context, contactID ids.ContactID, field, value string,
) error {
	in := contacts.UpdateContactInput{}
	switch field {
	case consent.ConfirmFieldFullName:
		in.FullName = &value
	case consent.ConfirmFieldTitle:
		in.Title = &value
	default:
		return consent.ErrUnsupportedField
	}
	_, err := a.store.UpdateContact(ctx, contactID, in)
	return err
}
