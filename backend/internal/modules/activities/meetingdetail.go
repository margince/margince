// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func attachMeetingParticipants(ctx context.Context, tx pgx.Tx, activity *crmcontracts.Activity) error {
	if activity.Kind != crmcontracts.ActivityKindMeeting || activity.ContentState != nil && *activity.ContentState == crmcontracts.ActivityContentStateWithheld {
		return nil
	}
	if err := auth.Require(ctx, linkEntityActivity, principal.ActionRead); err != nil {
		return err
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	id := arg(activity.Id)
	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return err
	}
	columns, err := MeetingRoomColumns(ctx, arg)
	if err != nil {
		return err
	}
	var dealID *ids.UUID
	var encoded []byte
	err = tx.QueryRow(ctx, fmt.Sprintf(`SELECT %s FROM activity a WHERE a.id = $%d AND %s`, columns, id, orUnbounded(scope)), args...).Scan(&dealID, &encoded)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("activities: reading meeting participants: %w", err)
	}
	var participants []crmcontracts.MeetingParticipant
	if err := json.Unmarshal(encoded, &participants); err != nil {
		return fmt.Errorf("activities: decoding meeting participants: %w", err)
	}
	activity.MeetingParticipants = &participants
	return nil
}
