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

// DataTaskOperatorsTx uses current stored grants, so a revoked operator receives no correction notice.
func DataTaskOperatorsTx(ctx context.Context, tx pgx.Tx) ([]ids.UserID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	users, grants, err := liveOperatorGrantsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := make([]ids.UserID, 0, len(users))
	for _, user := range users {
		if grants[user].perms.Allows("data_task", principal.ActionRead) {
			out = append(out, ids.From[ids.UserKind](user))
		}
	}
	return out, nil
}

func liveOperatorGrantsTx(ctx context.Context, tx pgx.Tx) ([]ids.UUID, map[ids.UUID]seatGrants, error) {
	rows, err := tx.Query(ctx, `SELECT DISTINCT u.id FROM app_user u
 JOIN role_assignment ra ON ra.user_id = u.id JOIN role r ON r.id = ra.role_id
 WHERE r.key IN ('admin', 'ops') AND `+workforce.LiveHumanSQL("u"))
	if err != nil {
		return nil, nil, err
	}
	users, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return nil, nil, err
	}
	grants, err := loadGrantsFor(ctx, tx, users)
	if err != nil {
		return nil, nil, err
	}
	return users, grants, nil
}

// PrivacyReviewOperatorsTx selects live human controllers with current read access.
func PrivacyReviewOperatorsTx(ctx context.Context, tx pgx.Tx, object string) ([]ids.UserID, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, err
	}
	users, grants, err := liveOperatorGrantsTx(ctx, tx)
	if err != nil {
		return nil, err
	}
	var out []ids.UserID
	for _, user := range users {
		p := grants[user].perms
		if p.RowScope != principal.RowScopeAll || len(p.FieldMasks) > 0 || !p.Allows("contact", principal.ActionDelete) || !p.Allows(object, principal.ActionRead) {
			continue
		}
		if object == "knowledge_document" && !p.Allows("knowledge_corpus", principal.ActionRead) {
			continue
		}
		out = append(out, ids.From[ids.UserKind](user))
	}
	return out, nil
}

// LiveReviewRecipientTx validates the owner of a retained personal setting.
func LiveReviewRecipientTx(ctx context.Context, tx pgx.Tx, id ids.UUID) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var live bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user u WHERE u.id=@id AND `+workforce.LiveHumanSQL("u")+`)`, pgx.StrictNamedArgs{"id": id}).Scan(&live)
	return live, err
}
