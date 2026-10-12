// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/settings"
)

const (
	followUpMinDays = 1
	followUpMaxDays = 30
)

// FollowUpAfterDays is how long a message the reader sent may go unanswered
// before their worklist reminds them to follow up. An admin sets it for the
// whole workspace; two days is where a sales follow-up is due.
var FollowUpAfterDays = settings.Define[int](
	"activities.follow_up_after_days", "installation_settings", "update", 2,
	func(days int) error {
		if days < followUpMinDays || days > followUpMaxDays {
			return fmt.Errorf("follow up after %d..%d days", followUpMinDays, followUpMaxDays)
		}
		return nil
	},
).MachineryApplied() // the worklist applies it to rows the reader is separately gated for

// Definitions is activities's contribution to the settings registry; compose
// concatenates each module's list.
func Definitions() []settings.Definition {
	return []settings.Definition{FollowUpAfterDays}
}

// followUpAfterDays reads the setting inside the caller's transaction.
func followUpAfterDays(ctx context.Context, tx pgx.Tx) (int, error) {
	days, err := settings.ApplyTx(ctx, tx, FollowUpAfterDays)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", FollowUpAfterDays.Key(), err)
	}
	return days, nil
}
