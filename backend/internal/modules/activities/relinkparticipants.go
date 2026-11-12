// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// RelinkParticipantChange distinguishes a moved participant from one already present on the target.
type RelinkParticipantChange struct {
	ID            ids.UUID
	ContactID     ids.UUID
	Role          string
	UserID        *ids.UUID
	Address       *string
	ChannelUserID *string
	Move          bool
}

// A displaced contact's `from` row is never carried to the relink target: the
// target becomes the author only on the proof stampProvenSender applies. A row
// with a stated address or account keeps it as a bare sender
// (unbindDisplacedSenders); one without is carried as a copy.
const (
	statedSenderRowSQL = `ap.role = 'from' AND (ap.address IS NOT NULL OR ap.channel_user_id IS NOT NULL)`
	carriedRoleSQL     = `CASE WHEN ap.role = 'from' THEN 'cc' ELSE ap.role END`
)

// displacedRowsSQL selects the participant rows of the displaced contacts this
// caller can see, leaving out the relink target's own rows.
func displacedRowsSQL(ctx context.Context, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) (string, []any, int, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idPos, targetPos, displacedPos := arg(id), arg(target), arg(displaced)
	visible, err := auth.ScopeClauseFor(ctx, linkEntityContact, "op", arg)
	if err != nil {
		return "", nil, 0, err
	}
	if visible == "" {
		visible = "true"
	}
	where := storekit.SQLf(`ap.activity_id = $%d AND ap.contact_id = ANY($%d::uuid[]) AND ap.contact_id <> $%d
   AND EXISTS (SELECT 1 FROM contact op WHERE op.id = ap.contact_id AND (%s))`, idPos, displacedPos, targetPos, visible)
	return where, args, targetPos, nil
}

func participantRelinkQuery(ctx context.Context, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) (string, []any, int, error) {
	where, args, targetPos, err := displacedRowsSQL(ctx, id, target, displaced)
	if err != nil {
		return "", nil, 0, err
	}
	query := storekit.SQLf(`WITH scoped AS (
  SELECT ap.id, ap.activity_id, ap.contact_id, `+carriedRoleSQL+` AS role, ap.user_id, ap.address, ap.channel_user_id,
   row_number() OVER (
    PARTITION BY `+carriedRoleSQL+`, ap.user_id, ap.address, ap.channel_user_id
    ORDER BY ap.id) AS rank
  FROM activity_participant ap
  WHERE %s AND NOT (`+statedSenderRowSQL+`)
 ) SELECT s.id, s.contact_id, s.role, s.user_id, s.address, s.channel_user_id,
  s.rank = 1 AND NOT EXISTS (
   SELECT 1 FROM activity_participant other
   WHERE other.activity_id = s.activity_id AND other.role = s.role
    AND other.contact_id = $%d
    AND other.user_id IS NOT DISTINCT FROM s.user_id
    AND other.address IS NOT DISTINCT FROM s.address
    AND other.channel_user_id IS NOT DISTINCT FROM s.channel_user_id) AS move
 FROM scoped s`, where, targetPos)
	return query, args, targetPos, nil
}

// unbindDisplacedSenders turns a displaced contact's `from` row that states an
// address or account back into a bare sender row. A row whose bare twin already
// exists is deleted instead, as the unique index allows only one.
func unbindDisplacedSenders(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) error {
	where, args, _, err := displacedRowsSQL(ctx, id, target, displaced)
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, storekit.SQLf(`WITH stated AS (
  SELECT ap.id, row_number() OVER (PARTITION BY ap.user_id, ap.address, ap.channel_user_id ORDER BY ap.id) > 1
   OR EXISTS (SELECT 1 FROM activity_participant twin
    WHERE twin.activity_id = ap.activity_id AND twin.role = 'from' AND twin.contact_id IS NULL
     AND twin.user_id IS NOT DISTINCT FROM ap.user_id
     AND twin.address IS NOT DISTINCT FROM ap.address
     AND twin.channel_user_id IS NOT DISTINCT FROM ap.channel_user_id) AS twinned
  FROM activity_participant ap
  WHERE %s AND `+statedSenderRowSQL+`
 ), unbound AS (
  UPDATE activity_participant ap SET contact_id = NULL
  FROM stated s WHERE ap.id = s.id AND NOT s.twinned
  RETURNING ap.id
 ) DELETE FROM activity_participant WHERE id IN (SELECT id FROM stated WHERE twinned)`, where), args...)
	return err
}

func previewRelinkParticipants(ctx context.Context, tx pgx.Tx, id ids.ActivityID, target ids.UUID, displaced []ids.UUID) ([]RelinkParticipantChange, error) {
	query, args, _, err := participantRelinkQuery(ctx, id, target, displaced)
	if err != nil {
		return nil, err
	}
	rows, err := tx.Query(ctx, query+" ORDER BY s.id", args...)
	if err != nil {
		return nil, err
	}
	changes, err := pgx.CollectRows(rows, pgx.RowToStructByPos[RelinkParticipantChange])
	if err != nil {
		return nil, err
	}
	if err := EnsureParticipantRosterVisibleTx(ctx, tx, id, slices.ContainsFunc(changes, func(change RelinkParticipantChange) bool { return change.Role == roleBcc })); err != nil {
		return nil, err
	}
	return changes, nil
}

// EnsureParticipantRosterVisibleTx keeps BCC membership behind the sender-seat check.
func EnsureParticipantRosterVisibleTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, hasBCC bool) error {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.EnsureActivityContentVisible(ctx, tx, id.UUID); err != nil {
		return err
	}
	if !hasBCC {
		return nil
	}
	sender, err := callerIsSenderSeat(ctx, tx, id)
	if err != nil {
		return err
	}
	if !sender {
		return apperrors.ErrNotFound
	}
	return nil
}
