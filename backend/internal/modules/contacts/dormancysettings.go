// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/kernel/dormancy"
)

// DormancyDays is the installation's dormancy ladder: the silent day on which
// cooling, dormant and stale begin. One setting, so a save cannot leave the
// three out of order.
var DormancyDays = config.DefineStored(config.Spec[[]int]{
	Key:     "contacts.dormancy_days",
	Env:     "MARGINCE_CONTACTS_DORMANCY_DAYS",
	Type:    config.List(config.Int),
	Default: config.Some(dormancy.DefaultDays()),
	Validate: func(days []int) error {
		_, err := dormancy.FromDays(days)
		return err
	},
	Doc: "Silent days after which an account reads as cooling, dormant and stale, in that order.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the ladder rides company reads the caller is already gated for
})

// DormancyFollowUpEnabled is whether a dormant or stale account suggests a
// follow-up task to its owner. On by default; off keeps the ladder and drops
// only the suggestion.
var DormancyFollowUpEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "contacts.dormancy_follow_up_enabled",
	Env:     "MARGINCE_CONTACTS_DORMANCY_FOLLOW_UP_ENABLED",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Suggest a follow-up task to the owner of an account that has gone dormant or stale.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the company page applies it to a suggestion card it already gates
})

// DormancyPolicy is the ladder and the follow-up switch, read together.
type DormancyPolicy struct {
	Ladder   dormancy.Ladder
	FollowUp bool
}

// DormancyPolicyTx reads both settings in one statement.
func DormancyPolicyTx(ctx context.Context, tx pgx.Tx) (DormancyPolicy, error) {
	days, followUp, err := config.StoredApplyPairTx(ctx, tx, config.Bound(), DormancyDays, DormancyFollowUpEnabled)
	if err != nil {
		return DormancyPolicy{}, fmt.Errorf("contacts: reading the dormancy settings: %w", err)
	}
	ladder, err := dormancy.FromDays(days)
	if err != nil {
		return DormancyPolicy{}, fmt.Errorf("contacts: the stored dormancy days: %w", err)
	}
	return DormancyPolicy{Ladder: ladder, FollowUp: followUp}, nil
}
