// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// machineSenderDataTask marks the received mail captured before capture
// stamped sender_is_machine, by the same rule capture stamps it with. The
// stored contact edges that counted such mail are folded again in the same
// transaction; strength and the last-touch clocks read the column live.
func machineSenderDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792795558_sending_system_mail_marked", Version: 2,
		Title: "Marking mail a sending system sent", Requires: []string{"core/1792795558"},
		Steps: []datatask.Step{{
			Name: "received_mail", Limit: 200, MinLimit: 10,
			Audit:    datatask.AuditPolicy{Reason: "capture stamps the same sender judgement on every new mail without an audit"},
			Estimate: estimateInboundSenders, Batch: markMachineSendersBatch,
		}},
	}
}

func markMachineSendersBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	senders, err := activities.InboundSendersTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(senders))
	var machine []ids.UUID
	for _, sender := range senders {
		scanned = append(scanned, sender.ID)
		if capture.IsMachineAddress(sender.Address) {
			machine = append(machine, sender.ID)
		}
	}
	marked, err := activities.MarkMachineSendersTx(ctx, tx, machine)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	if err := search.RecomputeEdgesForActivities(ctx, tx, marked); err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, len(marked))
}

func estimateInboundSenders(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountInboundSenders(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
