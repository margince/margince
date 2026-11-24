// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package rolemailboxseats is the data task that takes a seat off the answer of
// a role mailbox on a meeting. A calendar once bound such an answer to the
// member who connected the mailbox, which credited that member with attending.
package rolemailboxseats

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask applies the rule a calendar roster applies today: only a member's
// own answer binds them to a meeting.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795390756_role_mailbox_answers_unbound", Version: 1,
		Title: systemtext.Literal("datatask.title.core.1795390756_role_mailbox_answers_unbound"), Requires: []string{"core/1788151532"},
		Steps: []datatask.Step{{
			Name: "unbind_role_mailbox_seats", Limit: 200, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimate, Batch: batch,
		}},
	}
}

func batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := activities.RoleMailboxActivitiesTx(ctx, tx, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, meeting := range found {
		unbound, err := activities.UnbindRoleMailboxSeatsTx(ctx, tx, meeting, in.RunID)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if unbound {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, found, changed)
}

func estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	count, err := activities.CountRoleMailboxActivities(ctx, q, maximum)
	return min(count, maximum), count > maximum, err
}
