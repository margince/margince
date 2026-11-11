// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package provensenders is the data task that names the sender of received
// messages stored without one. It names a linked contact proven to have written
// the message.
package provensenders

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask applies the rule a later link applies (activities.StampProvenSenderTx),
// so a group member the address does not prove is never made the author.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1794374353_proven_message_senders", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1794374353_proven_message_senders"), Requires: []string{"core/1788759372"},
		Steps: []datatask.Step{{
			Name: "stamp_proven_senders", Limit: 200, MinLimit: 1,
			Audit:         datatask.AuditPolicy{PerRecord: true},
			EmptyWhenDone: true,
			Estimate:      estimateProvenSenders, Batch: stampProvenSendersBatch,
		}},
	}
}

func stampProvenSendersBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.ProvenSenderActivitiesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, activityID := range found {
		stamped, err := activities.StampProvenSenderTx(ctx, tx, activityID, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if stamped {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, found, changed)
}

func estimateProvenSenders(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountProvenSenderActivities(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
