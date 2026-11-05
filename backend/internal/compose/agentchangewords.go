// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/platform/database"
)

// agentChangeWords names a tool write's fields with the approval inbox's
// engine. So the chat and the inbox show one change in the same words: a
// seat holder by name, an amount in its currency.
func agentChangeWords(db *database.DB) agents.ChangeWords {
	reader := newApprovalPreviewReader(db)
	return func(ctx context.Context, current, changes map[string]json.RawMessage) ([]crmcontracts.ApprovalPreviewChange, error) {
		return reader.changedValues(ctx, toolUpdateRecord, current, changes)
	}
}
