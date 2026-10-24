// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/credentialmask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// auditScheduledSendTx writes the audit row of one change to a scheduled send.
// The payload keeps the text as typed because it is what goes out, while the
// audit log reaches readers the sender never chose, so every text in its images
// is masked.
func auditScheduledSendTx(ctx context.Context, tx pgx.Tx, action string, id ids.UUID, before, after map[string]any) error {
	_, err := storekit.Audit(ctx, tx, action, "scheduled_send", id, maskedAuditImage(before), maskedAuditImage(after))
	return err
}

func maskedAuditImage(image map[string]any) map[string]any {
	if image == nil {
		return nil
	}
	masked := make(map[string]any, len(image))
	for field, value := range image {
		if text, ok := value.(string); ok {
			value = credentialmask.MaskText(text)
		}
		masked[field] = value
	}
	return masked
}
