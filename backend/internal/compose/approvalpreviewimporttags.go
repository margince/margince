// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func (r approvalPreviewReader) importRowTags(ctx, execCtx context.Context, writer *csvWriters, row migration.Row, creates bool, out *crmcontracts.ApprovalPreviewImportRow) error {
	entityType, taggable := taggableObjectOf(writer.object)
	if !taggable {
		return nil
	}
	if creates && !writer.contextTag.IsZero() {
		value, err := r.referenceValue(ctx, "tag_id", writer.contextTag.String())
		if err != nil {
			return err
		}
		out.Details = append(out.Details, crmcontracts.ApprovalPreviewDetail{Field: "import_context_tag", Value: value})
	}
	if !auth.Allows(execCtx, entityType, principal.ActionUpdate) {
		return nil
	}
	for _, name := range splitTags(textFields(row.Fields)[csvTargetTags]) {
		value := readPreviewText(name)
		if tag, exists := writer.tagNames.byName[tagKey(name)]; exists {
			var err error
			value, err = r.referenceValue(ctx, "tag_id", tag.String())
			if err != nil {
				return err
			}
		}
		out.Details = append(out.Details, crmcontracts.ApprovalPreviewDetail{Field: "import_apply_tag", Value: value})
	}
	return nil
}
