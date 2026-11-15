// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package qualifytasks

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// SystemTextDataTask restores catalog descriptors without changing user wording.
func SystemTextDataTask() datatask.Task {
	sources := slices.DeleteFunc(storekit.TaskSourceFamilies(), func(source string) bool { return source == provenance.LeadRepliedSource })
	return datatask.Task{
		ID: "core/1794739855_system_task_text", Version: 2,
		Title: systemtext.Literal("datatask.title.core.1794739855_system_task_text"), Requires: []string{"core/1794283985"},
		Steps: []datatask.Step{{
			Name: "restore_task_instructions", Limit: 100, MinLimit: 1, Audit: datatask.AuditPolicy{PerRecord: true},
			Estimate: func(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
				return activities.CountLegacyTaskText(ctx, q, maximum, sources)
			},
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				tasks, err := activities.LegacySystemTasksTx(ctx, tx, after, in.Limit, sources)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				scanned := make([]ids.UUID, 0, len(tasks))
				changed := 0
				for _, task := range tasks {
					scanned = append(scanned, task.ID)
					message, err := legacyTaskMessage(task)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					if message == nil {
						continue
					}
					applied, err := activities.RestoreTaskSystemTextTx(ctx, tx, task, message)
					if err != nil {
						return datatask.BatchOut{}, err
					}
					if applied {
						changed++
					}
				}
				return datatask.AdvanceUUID(in, scanned, changed)
			},
		}},
	}
}

func legacyTaskMessage(task activities.LegacySystemTask) (*systemtext.Message, error) {
	subject, err := systemtext.RecoverCanonicalText(task.Subject)
	if err != nil || subject == nil {
		return nil, err
	}
	body, err := systemtext.RecoverCanonicalText(task.Body)
	if err != nil {
		return nil, err
	}
	return &systemtext.Message{Subject: subject, Body: body}, nil
}
