// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
)

const (
	followUpMinDays = 1
	followUpMaxDays = 30
	// settingsAuditVerb is the audit action every activities settings change records.
	settingsAuditVerb = "update"
)

// FollowUpAfterDays is how long a message the reader sent may go unanswered
// before their worklist reminds them to follow up. An admin sets it for the
// whole workspace; two days is where a sales follow-up is due.
var FollowUpAfterDays = config.DefineStored(config.Spec[int]{
	Key:     "activities.follow_up_after_days",
	Env:     "MARGINCE_ACTIVITIES_FOLLOW_UP_AFTER_DAYS",
	Type:    config.Int,
	Default: config.Some(2),
	Min:     config.Some(followUpMinDays),
	Max:     config.Some(followUpMaxDays),
	Doc:     "Days a sent message may go without a reply before its sender is asked to follow up.",
}, config.Store{
	RBACObject: "installation_settings", AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the worklist applies it to rows the reader is separately gated for
})

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{FollowUpAfterDays}
}

// followUpAfterDays reads the setting inside the caller's transaction, under
// the boot value the program bound.
func followUpAfterDays(ctx context.Context, tx pgx.Tx) (int, error) {
	days, err := config.StoredApplyTx(ctx, tx, config.Bound(), FollowUpAfterDays)
	if err != nil {
		return 0, fmt.Errorf("read %s: %w", FollowUpAfterDays.Key(), err)
	}
	return days, nil
}
