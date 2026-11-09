// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/workforce"
)

// EnsureInternalMailParticipantTx checks actual attendance before an individual share.
func EnsureInternalMailParticipantTx(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	actor, err := rbacActor(ctx)
	if err != nil {
		return err
	}
	if !actor.HumanOrItsAgent() {
		return apperrors.ErrPermissionDenied
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	row := arg(id)
	share, err := ActivityInternalMailShareClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	clause := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity a WHERE a.id = $%d AND %s)`, row, share)
	var participant bool
	if err := tx.QueryRow(ctx, clause, args...).Scan(&participant); err != nil {
		return err
	}
	if !participant {
		return apperrors.ErrPermissionDenied
	}
	return nil
}

// ActivityInternalMailShareClause supplies the same capability to readers and the write gate.
func ActivityInternalMailShareClause(ctx context.Context, alias string, arg func(any) int) (string, error) {
	actor, err := rbacActor(ctx)
	if err != nil {
		return "", err
	}
	if !actor.HumanOrItsAgent() || !actor.SeatType.CanMutate() || !actor.Permissions.Allows("activity", principal.ActionUpdate) {
		return "false", nil
	}
	me := arg(actor.UserID)
	return fmt.Sprintf(`(%[1]s.internal_mail_authorization IS NOT NULL AND %[1]s.archived_at IS NULL
 AND %[1]s.restricted_at IS NULL AND EXISTS (
 SELECT 1 FROM app_user internal_reader WHERE internal_reader.id = $%[2]d
 AND %[3]s AND %[4]s))`, alias, me, workforce.LiveHumanSQL("internal_reader"), activityAttendanceForMemberSQL(alias, memberIdentityExpressionSQL("internal_reader.id"))), nil
}
