// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The litigation-hold writer for the three record tables this module owns.
//
// The flag was read everywhere and written nowhere: every retention selector
// carries `NOT legal_hold` and the Art. 17 cascade refuses a held record, so
// the guard existed while no hold could ever be placed. This is the half that
// was missing, and the shape of the write is storekit.SetLegalHold — shared
// with deals and projects, which own the other two tables.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// HeldTable names one of this module's three holdable tables. A closed type
// rather than a string, so the entity a caller asks about is checked where it
// is CONSTRUCTED and never carried as free text into a statement.
type HeldTable string

// The three tables, one constant each. A caller names the record it means and
// the statement is built from this and nothing else.
const (
	HeldContact HeldTable = "contact"
	HeldCompany HeldTable = "company"
	HeldLead    HeldTable = "lead"
)

// SetLegalHold places or lifts the hold on one contact, company or lead.
//
// Gated as a DELETE on the object, not an update: a hold decides whether a
// record can be erased at all, so the authority that governs erasure is the
// one that should govern its suspension. An editor who may change a name has
// no business overriding the storage-limitation ladder.
func (s *Store) SetLegalHold(
	ctx context.Context, table HeldTable, id ids.UUID, held bool, reason string,
) error {
	object := string(table)
	if err := auth.Require(ctx, object, principal.ActionDelete); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureWritable(ctx, tx, object, id); err != nil {
			return err
		}
		switch table {
		case HeldContact, HeldCompany, HeldLead:
			return storekit.SetLegalHold(ctx, tx, object, id, held, reason)
		default:
			// Unreachable through the exported constants, and a fail-closed
			// answer rather than a statement built from an unknown name.
			return fmt.Errorf("legal hold on %q: %w", table, apperrors.ErrInvalidArgument)
		}
	})
}
