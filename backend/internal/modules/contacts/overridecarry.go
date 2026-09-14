// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// What happens to a subject's standing OVERRIDES when a merge retires them —
// StopCarrier's other half, split out of stopcarry.go so each carry's own
// reasoning stays legible on its own rather than doubling one file.
//
// A standing override (consent/override.go's Allow) is a rep vouching that a
// machine-level refusal may be overruled for one category. Left uncarried, it
// is exactly as quiet a loss as an uncarried stop: the merge moves everything
// else that points at the retiring record, the override stays attached to an
// id the send engine no longer evaluates, and the next send to the survivor
// for that category is refused again — with nothing on file saying a rep
// already vouched for it.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// carryOverridesTx is the one call site, so the refusal below cannot be
// forgotten by a second caller written later — StopCarrier's own doc note for
// carryStopsTx, held here for the same reason.
func (s *Store) carryOverridesTx(ctx context.Context, tx pgx.Tx, from, to commsauthz.StopSubject) error {
	return carryOverridesOrRefuse(ctx, tx, s.stopCarrier, from, to)
}

// carryOverridesOrRefuse mirrors carryStopsOrRefuse (stopcarry.go): an unwired
// carrier refuses the merge only when the retiring subject holds an override
// the merge would otherwise drop, exactly the data-conditioned refusal
// carryStopsOrRefuse's own comment argues for — refusing every merge outright
// would break every caller with nothing to do with consent, and refusing
// nothing would restore the silent-drop defect this file exists to close.
func carryOverridesOrRefuse(ctx context.Context, tx pgx.Tx, carrier StopCarrier, from, to commsauthz.StopSubject) error {
	if carrier != nil {
		return carrier.CarryOverridesTx(ctx, tx, from, to)
	}
	held, err := holdsALiveOverride(ctx, tx, from)
	if err != nil {
		return err
	}
	if held {
		return &OverrideCarrierNotWiredError{}
	}
	return nil
}

// holdsALiveOverride is holdsALiveStop's sibling: a read-only fact about a
// table this module does not own, asked only to decide whether an unwired
// merge is about to destroy something.
func holdsALiveOverride(ctx context.Context, tx pgx.Tx, subject commsauthz.StopSubject) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM communication_override
			 WHERE revoked_at IS NULL
			   AND (($1::uuid IS NOT NULL AND contact_id = $1)
			     OR ($2::uuid IS NOT NULL AND lead_id = $2)))`,
		zeroAsNull(subject.ContactID.UUID), zeroAsNull(subject.LeadID.UUID)).Scan(&held)
	if err != nil {
		return false, fmt.Errorf("contacts: checking whether the retiring record holds an override: %w", err)
	}
	return held, nil
}

// OverrideCarrierNotWiredError maps to 422, mirroring StopCarrierNotWiredError:
// nothing is broken, but this installation cannot merge this subject safely
// until the seam that would carry the override onto the survivor is wired.
type OverrideCarrierNotWiredError struct{}

func (e *OverrideCarrierNotWiredError) Error() string {
	return "this record carries a recorded override and the consent seam that would move it onto the " +
		"survivor is not wired on this installation; merging would silently drop a rep's vouch for it"
}

// FieldFault carries the refusal to every surface, mirroring
// StopCarrierNotWiredError.FieldFault for the same reason: the MCP tool
// surface reaches this store through the datasource seam and never runs the
// REST error mapper, and the field names the source record because that is
// the one holding the override and the one an operator will look at.
func (e *OverrideCarrierNotWiredError) FieldFault() (field, code, message string) {
	return "source_id", "override_carrier_not_wired", e.Error()
}
