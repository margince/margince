// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// LiveSeatsAllowingTx lists the live human seats whose current roles grant every
// one of the actions on the object. It reads stored grants, so a seat whose role
// changed a moment ago is judged by the new one. Only system work may ask: it
// names who could act, which no seat's own request is entitled to.
func LiveSeatsAllowingTx(ctx context.Context, tx pgx.Tx, object string, actions ...principal.Action) ([]ids.UserID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, `SELECT u.id FROM app_user u WHERE `+workforce.LiveHumanSQL("u")+` ORDER BY u.id`)
	if err != nil {
		return nil, err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, err
	}
	grants, err := loadGrantsFor(ctx, tx, users)
	if err != nil {
		return nil, err
	}
	var out []ids.UserID
	for _, user := range users {
		allowed := true
		for _, action := range actions {
			allowed = allowed && grants[user].perms.Allows(object, action)
		}
		if allowed {
			out = append(out, ids.From[ids.UserKind](user))
		}
	}
	return out, nil
}
