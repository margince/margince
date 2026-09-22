// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The mask a reader reads ONE record's trail under. It is the live read's mask,
// resolved for that record: hiding history and value is one motion, and a
// reader who may read the figure today has no past of it to be kept from.
//
// Evaluated NOW, never per audit row. A reader who gained the record last month
// sees the whole trail, the period before they could have included — the
// alternative is a second mechanism deciding what the live read already decided.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// maskForRecord answers the fields of one record whose history is withheld from
// the caller, each configured mask's group already expanded by auth.
func maskForRecord(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) (entityFieldMask, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return nil, apperrors.ErrPermissionDenied
	}
	// The widest answer first, because it is free. Only a mask conditioned on
	// write authority narrows under the lifted one, so only a reader who carries
	// one pays the statement that resolves the row's write arm.
	withheld := auth.MaskedFields(actor, entityType, false)
	if lifted := auth.MaskedFields(actor, entityType, true); len(lifted) < len(withheld) {
		writable, err := recordIsWritable(ctx, tx, entityType, entityID)
		if err != nil {
			return nil, err
		}
		if writable {
			withheld = lifted
		}
	}
	mask := make(entityFieldMask, len(withheld))
	for _, field := range withheld {
		mask[field] = struct{}{}
	}
	return mask, nil
}

// recordIsWritable answers the arm a mask conditioned on write authority lifts
// on. An activity carries no owner and no grant, so the question has no answer
// there and the mask stays on — a condition that cannot be resolved withholds.
func recordIsWritable(ctx context.Context, tx pgx.Tx, entityType string, entityID ids.UUID) (bool, error) {
	if entityType == entityTypeActivity {
		return false, nil
	}
	writable, err := auth.WritableSubset(ctx, tx, entityType, []ids.UUID{entityID})
	if err != nil {
		return false, err
	}
	return writable[entityID], nil
}

// refuseMaskedFieldFilter refuses a field filter naming a field this reader's
// masks withhold. An empty page would be just as safe and would teach the
// reader the value never changed.
func refuseMaskedFieldFilter(field *string, mask entityFieldMask, entityType string) error {
	if field == nil {
		return nil
	}
	if _, withheld := mask[*field]; !withheld {
		return nil
	}
	return &values.ParseError{
		Field: "field", Code: auth.CodeFieldMasked,
		Message: "the history of " + *field + " is not available: your role does not read it on this " + entityType,
	}
}
