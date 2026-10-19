// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func proposalReadable(ctx context.Context, tx pgx.Tx, p principal.Principal, a row) (bool, error) {
	granted := requireDecisionGrants(p, a) == nil
	if !granted || withheldFromOtherSeats(p, a) {
		return false, nil
	}
	if len(a.Evidence) == 0 {
		return true, nil
	}
	var evidence []evidenceJSON
	if err := json.Unmarshal(a.Evidence, &evidence); err != nil {
		return false, fmt.Errorf("read proposal evidence: %w", err)
	}
	for _, item := range evidence {
		if item.SourceType == nil || *item.SourceType != "activity" {
			continue
		}
		visible, err := targetVisible(ctx, tx, item.SourceType, item.SourceID)
		if err != nil || !visible {
			return false, err
		}
	}
	return true, nil
}
