// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// The installation-wide bound on automatic application. Each rep's own mode per
// kind (approval_autonomy_policy) stays the choice; this switch only caps it.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
)

// installationSettingsObject is the grant that reads the installation's settings.
const installationSettingsObject = "installation_settings"

// settingsAuditVerb is the audit action a change of this module's settings records.
const settingsAuditVerb = "update"

// AutoApplyEnabled is the installation's outer bound on proposals that apply
// themselves. Off, no kind applies for anybody, stage moves and openings
// included. On, nothing changes for a rep: their own mode per kind still
// decides. Turning it back on never switches on a kind somebody turned off.
var AutoApplyEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "approvals.auto_apply_enabled",
	Env:     "MARGINCE_APPROVALS_AUTO_APPLY_ENABLED",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let a proposal apply itself where its owner has that kind on automatic. Off stops every automatic apply.",
}, config.Store{
	RBACObject: installationSettingsObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the sweeps decide for an owner; no request carries it
})

// AutoApplyEnabledTx is the one read of the switch, for the applier, the
// sweeps and the rep's settings screen, in the caller's transaction.
func AutoApplyEnabledTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	return config.StoredApplyTx(ctx, tx, config.Bound(), AutoApplyEnabled)
}

// SetAutoApplyEnabled switches automatic application for the installation. An
// admin's act alone: ops holds installation_settings.update for the wiring it
// runs, and whether the product may act unasked is not wiring.
func SetAutoApplyEnabled(ctx context.Context, cfg *config.Config, on bool) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	return config.Save(ctx, cfg, AutoApplyEnabled, on)
}

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{AutoApplyEnabled}
}
