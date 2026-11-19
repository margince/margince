// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The system read of who holds a role. A job tick has no caller, so a unit
// that files a task for each reviewer asks here. It reads the members the
// notice door would reach for the same role, and gives only their ids.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
	"github.com/margince/margince/backend/pkg/extension"
)

func (d systemDirectory) ByRole(ctx context.Context, roles []string) ([]extension.UserID, error) {
	ctx, err := d.rt.systemRead(ctx, extension.SystemReadDirectory)
	if err != nil {
		return nil, err
	}
	if len(roles) > extension.MaxResponsible {
		return nil, fmt.Errorf("%w: %d roles in one call, and the most is %d", extension.ErrInvalid, len(roles), extension.MaxResponsible)
	}
	if len(roles) == 0 {
		return []extension.UserID{}, nil
	}
	if err := d.rt.traceSystemRead(ctx, extension.SystemReadDirectory, "role", fmt.Sprintf("%d roles", len(roles))); err != nil {
		return nil, err
	}
	var members []ids.UUID
	err = database.WithWorkspaceTx(ctx, d.rt.deps.pool, func(tx pgx.Tx) error {
		var known int
		if err := tx.QueryRow(ctx, `SELECT count(DISTINCT key) FROM role WHERE key = ANY($1) AND archived_at IS NULL`, roles).Scan(&known); err != nil {
			return err
		}
		if known != len(uniqueKeys(roles)) {
			return extension.ErrInvalid
		}
		rows, err := tx.Query(ctx, `
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
		if errors.Is(err, extension.ErrInvalid) {
			return nil, fmt.Errorf("%w: a role key names no role", extension.ErrInvalid)
		}
		return nil, d.rt.systemReadFault(ctx, err)
	}
	out := make([]extension.UserID, len(members))
	for i, id := range members {
		out[i] = extension.UserID(id.String())
	}
	return out, nil
}

func uniqueKeys(keys []string) map[string]struct{} {
	set := make(map[string]struct{}, len(keys))
	for _, k := range keys {
		set[k] = struct{}{}
	}
	return set
}
