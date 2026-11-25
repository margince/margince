// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package participantaddresses is the data task that lowercases the addresses
// of participant rows written before the table held them lowercase.
package participantaddresses

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask lowercases each older address through the table's update trigger.
// Two spellings of one party on an activity and role become one row there.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795587300_participant_addresses_lowercase", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795587300_participant_addresses_lowercase"),
		Requires: []string{"core/1795587300", "core/1795587303"},
		Steps: []datatask.Step{{
			Name: "lowercase_addresses", Limit: 200, MinLimit: 1,
			Audit:         datatask.AuditPolicy{PerRecord: true},
			EmptyWhenDone: true,
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
					return 0, false, err
				}
				left, err := activities.CountParticipantAddressesToLowercase(ctx, q, maximum)
				return min(left, maximum), left > maximum, err
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				inspected, changed, err := activities.LowercaseParticipantAddressesTx(ctx, tx, after, in.Limit, in.RunID)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				return datatask.AdvanceUUID(in, inspected, changed)
			},
		}},
	}
}
