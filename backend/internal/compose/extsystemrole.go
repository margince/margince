// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of who holds a role. A job tick has no caller, so a unit
// that files a task for each reviewer asks here. It reads the members the
// notice door would reach for the same role, and gives only their ids.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

func (d systemDirectory) ByRole(ctx context.Context, actAs extension.UserID, roles []string) ([]extension.UserID, error) {
	seatCtx, seat, err := d.rt.seatRead(ctx, extension.SystemReadDirectory, actAs)
	if err != nil {
		return nil, err
	}
	if len(roles) > extension.MaxResponsible {
		return nil, fmt.Errorf("%w: %d roles in one call, and the most is %d", extension.ErrInvalid, len(roles), extension.MaxResponsible)
	}
	// Who holds which role is the administrators' view of the roster. A seat
	// that may not open it is refused, as it is in the app.
	if !auth.Allows(seatCtx, "user_admin", principal.ActionRead) {
		return nil, fmt.Errorf("%w: the seat may not read the roster of roles", extension.ErrForbidden)
	}
	if err := seat.trace("role", fmt.Sprintf("%d roles", len(roles)), append([]string{}, roles...)); err != nil {
		return nil, err
	}
	if len(roles) == 0 {
		return []extension.UserID{}, nil
	}
	var members []ids.UUID
	err = database.WithWorkspaceTx(seatCtx, d.rt.deps.pool, func(tx pgx.Tx) error {
		// A key that names no role matches no row, so it adds nobody and the
		// answer does not tell whether the role exists.
		rows, err := tx.Query(seatCtx, `
			SELECT DISTINCT u.id FROM app_user u
			  JOIN role_assignment ra ON ra.user_id = u.id
			  JOIN role r ON r.id = ra.role_id AND r.archived_at IS NULL
			 WHERE r.key = ANY($1) AND NOT u.is_agent AND `+workforce.LiveMemberSQL("u")+`
			 ORDER BY u.id LIMIT $2`, roles, extension.MaxResponsible)
		if err != nil {
			return err
		}
		members, err = pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		return err
	})
	if err != nil {
		return nil, seat.seatFault(err)
	}
	out := make([]extension.UserID, len(members))
	for i, id := range members {
		out[i] = extension.UserID(id.String())
	}
	return out, nil
}
