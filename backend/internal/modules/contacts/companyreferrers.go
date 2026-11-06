// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// StillReferencedError refuses the archive of a company that live records
// still name. It is a conflict, so an undo records the company as one it left
// in place rather than stopping.
type StillReferencedError struct{ Deals, Contacts int }

func (e *StillReferencedError) Error() string {
	return fmt.Sprintf("%d live deal(s) and %d live contact(s) still name this company, so it was left in place", e.Deals, e.Contacts)
}

// Is makes the refusal a conflict for errors.Is.
func (e *StillReferencedError) Is(target error) bool { return target == apperrors.ErrConflict }

// refuseIfReferenced counts the live records that name the company, inside the
// archive's own transaction. An archived deal does not count, and archiving a
// contact retires its employment edges, so a live edge is a live contact.
func refuseIfReferenced(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	return refuseIfNamedBy(ctx, tx, id, false)
}

// refuseIfReferencedByOthers is refuseIfReferenced over the records someone
// other than the company's owner owns. An owner archiving their own company
// without the delete grant would otherwise hide it from those owners' deals
// and retire their contacts' employment with it.
func refuseIfReferencedByOthers(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	return refuseIfNamedBy(ctx, tx, id, true)
}

func refuseIfNamedBy(ctx context.Context, tx pgx.Tx, id ids.CompanyID, othersOnly bool) error {
	var e StillReferencedError
	err := tx.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM deal d
		    WHERE d.archived_at IS NULL AND $1 IN (d.company_id, d.partner_company_id)
		      AND (NOT $2 OR d.owner_id IS DISTINCT FROM (SELECT c.owner_id FROM company c WHERE c.id = $1))),
		  (SELECT count(DISTINCT r.contact_id) FROM relationship r
		     JOIN contact ct ON ct.id = r.contact_id
		    WHERE r.kind = 'employment' AND r.company_id = $1 AND r.archived_at IS NULL
		      AND (NOT $2 OR ct.owner_id IS DISTINCT FROM (SELECT c.owner_id FROM company c WHERE c.id = $1)))`,
		id.UUID, othersOnly).Scan(&e.Deals, &e.Contacts)
	if err != nil {
		return fmt.Errorf("contacts: counting the live records that name company %s: %w", id, err)
	}
	if e.Deals+e.Contacts > 0 {
		return &e
	}
	return nil
}
