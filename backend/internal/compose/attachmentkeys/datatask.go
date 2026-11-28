// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package attachmentkeys binds older files to their existing records.
package attachmentkeys

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// DataTask binds older attachment owners without changing the files.
func DataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1795862828_attachment_owner_keys", Version: 1,
		Title:    systemtext.Literal("datatask.title.core.1795862828_attachment_owner_keys"),
		Requires: []string{"core/1795862831"},
		Steps: []datatask.Step{{
			Name: "key_attachment_owners", Limit: 500, MinLimit: 10,
			Audit: datatask.AuditPolicy{Reason: "the trigger keys new attachments, and deletion of an orphan follows the same ON DELETE CASCADE rule without an audit row"},
			// This step retains unresolved rows and reports their IDs while advancing.
			// keyPolymorphicRowsStep only accepts rows it can change.
			Estimate: activities.EstimateUnkeyedAttachmentOwners,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				if err := auth.RequireSystem(ctx); err != nil {
					return datatask.BatchOut{}, err
				}
				after, err := datatask.UUIDCursor(in.Cursor)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				page, err := activities.KeyAttachmentOwnersTx(ctx, tx, after, in.Limit)
				if err != nil {
					return datatask.BatchOut{}, err
				}
				out, err := datatask.AdvanceUUID(in, page.IDs, len(page.IDs)-len(page.NeedsReview))
				if len(page.NeedsReview) > 0 {
					out.Action = &datatask.ActionNeeded{Reason: "attachment_owner_unresolved"}
					for _, id := range page.NeedsReview {
						out.Action.Records = append(out.Action.Records, datatask.RecordRef{Type: "attachment", ID: id})
					}
				}
				return out, err
			},
		}},
	}
}
