// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// ResolveCalendarInvitation ensures provider echoes belong to the organizer's existing invitation, including the
// interval between provider acceptance and the delivery worker's receipt commit.
func ResolveCalendarInvitation(ctx context.Context, tx pgx.Tx, provider, event, request string) (ids.ActivityID, bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return ids.ActivityID{}, false, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ids.ActivityID{}, false, err
	}
	if err := refuseErasedInvitationEcho(ctx, tx, provider, event, request); err != nil {
		return ids.ActivityID{}, false, err
	}
	args := schedulingArgs{}
	host, p, e, r := args.add(actor.UserID), args.add(provider), args.add(event), args.add(strings.ReplaceAll(request, "-", ""))
	var id ids.ActivityID
	err := tx.QueryRow(ctx, `SELECT m.activity_id FROM meeting_invitation m JOIN activity a ON a.id=m.activity_id
 WHERE m.host_user_id=`+host+` AND m.provider=`+p+` AND a.archived_at IS NULL
 AND (m.receipt->>'event_id'=`+e+` OR (`+r+`<>'' AND replace(m.appointment->>'RequestID','-','')=`+r+`))`, args...).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.ActivityID{}, false, nil
	}
	if err != nil {
		return ids.ActivityID{}, false, err
	}
	if err := auth.EnsureActivityVisible(ctx, tx, id.UUID); err != nil {
		return ids.ActivityID{}, false, err
	}
	return id, true, nil
}

func refuseErasedInvitationEcho(ctx context.Context, tx pgx.Tx, provider, event, request string) error {
	args := schedulingArgs{}
	p, e, r := args.add(provider), args.add(event), args.add(strings.ReplaceAll(request, "-", ""))
	var erased bool
	err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM meeting_invitation WHERE status='erased' AND provider=`+p+` AND
 (receipt->>'event_id'=`+e+` OR (`+r+`<>'' AND replace(appointment->>'RequestID','-','')=`+r+`)))`, args...).Scan(&erased)
	if err != nil {
		return err
	}
	if erased {
		return fmt.Errorf("calendar invitation erased: %w", connector.ErrSkip)
	}
	return nil
}
