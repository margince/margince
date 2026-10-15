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
// archive's own transaction. Archived deals and contacts do not count: they no
// longer answer a list.
func refuseIfReferenced(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	var e StillReferencedError
	err := tx.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM deal d
		    WHERE d.archived_at IS NULL AND $1 IN (d.company_id, d.partner_company_id)),
		  (SELECT count(DISTINCT r.contact_id) FROM relationship r
		     JOIN contact c ON c.id = r.contact_id AND c.archived_at IS NULL
		    WHERE r.kind = 'employment' AND r.company_id = $1 AND r.archived_at IS NULL)`,
		id.UUID).Scan(&e.Deals, &e.Contacts)
	if err != nil {
		return fmt.Errorf("contacts: counting the live records that name company %s: %w", id, err)
	}
	if e.Deals+e.Contacts > 0 {
		return &e
	}
	return nil
}
