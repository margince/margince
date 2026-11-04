// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deals module's own settings declarations.
//
// Both entries are kill switches rather than preferences: the installation's
// answer to whether stage automation may move a deal, and whether the nightly
// maintenance sweep may write to one.

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/config"
)

// StageAutopilotEnabled is the installation's switch for Margince moving
// deals to a later stage by itself. On by default, like each owner's own
// switch: an admin's stored "off" stays off, and stops every move.
//
// Read inside the transaction that moves the deal, so turning it off stops
// the next move rather than the one after.
var StageAutopilotEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "deals.stage_autopilot_enabled",
	Env:     "MARGINCE_DEALS_STAGE_AUTOPILOT_ENABLED",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let Margince move a deal to a later stage when the evidence on the deal shows it reached that stage; off stops every automatic move.",
}, config.Store{
	RBACObject: stageAutomationObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the apply path reads it; no request carries it
})

// stageAutomationObject is the RBAC object gating stage automation's
// governance. `pipeline`, because that is what a rule is about — the same
// grant that lets an admin reshape a pipeline's stages lets them say whether
// those stages move themselves, and a separate object would be a second
// answer to who owns a pipeline.
const stageAutomationObject = "pipeline"

// MaintenanceWritesEnabled is the installation-wide answer to whether the
// nightly close-date sweep may write to a deal at all.
//
// DEFAULT TRUE, unlike its sibling above, because the sweep already ships on: a
// switch that defaulted to off would silently stop work an installation is
// relying on the moment it appeared. What this entry buys is a way to STOP the
// sweep without a redeploy — the tier it governs was a compile-time constant,
// so halting a misbehaving pass meant shipping a new binary.
//
// It gates EVERY automatic write the sweep makes: the direct re-date, the
// provisional write and the downgrade. A switch that stopped one branch while
// two others kept writing would not be a kill switch, and an operator reaching
// for it is not distinguishing between tiers.
//
// Read INSIDE each write's own transaction, for the reason
// StageAutopilotModeTx gives: read once at the top of a pass instead, it would
// authorize writes on an answer that was true when the pass started.
//
// Assessment, the receipt and Undo stay outside its reach on purpose. Switching
// maintenance off stops NEW changes; it does not hide the changes already made
// or take away a reader's ability to reverse one.
var MaintenanceWritesEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "deals.maintenance_writes_enabled",
	Env:     "MARGINCE_DEALS_MAINTENANCE_WRITES_ENABLED",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let the nightly close-date sweep write to a deal. Off stops every automatic write the sweep makes.",
}, config.Store{
	RBACObject: maintenanceObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the sweep reads it; no request carries it
})

// maintenanceObject is the RBAC object gating maintenance governance. `deal`,
// because a deal is what the sweep writes — the grant that lets somebody update
// a deal is the one that lets them say whether the machine may.
const maintenanceObject = "deal"

// LegalReviewEnabled is the installation's answer to whether a deal can be
// sent to legal for review, and whether a legal_reviewed criterion blocks an
// advance.
//
// DEFAULT TRUE, so teams find the feature without looking for it. Off hides
// the request, the queue and the criterion kind, and keeps every review
// already recorded. It is read inside the advance transaction, so switching it
// off releases a gated stage on the next move.
var LegalReviewEnabled = config.DefineStored(config.Spec[bool]{
	Key:     "deals.legal_review_enabled",
	Env:     "MARGINCE_DEALS_LEGAL_REVIEW_ENABLED",
	Type:    config.Bool,
	Default: config.Some(true),
	Doc:     "Let a deal be sent to legal for review, and let a legal_reviewed criterion hold a deal at its stage.",
}, config.Store{
	RBACObject: "installation_settings", AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the advance gate reads it for whoever moves the deal
})

// SilentCloseDays is how many days without word from the customer Margince
// waits before it closes an open deal as lost. Each owner can switch the close
// off for their own deals; this is the installation's threshold for everyone.
var SilentCloseDays = config.DefineStored(config.Spec[int]{
	Key:     "deals.silent_close_days",
	Env:     "MARGINCE_DEALS_SILENT_CLOSE_DAYS",
	Type:    config.Int,
	Default: config.Some(120),
	Min:     config.Some(StalledThresholdDays),
	Max:     config.Some(730),
	Doc:     "Days without word from the customer after which Margince closes an open deal as lost.",
}, config.Store{
	RBACObject: maintenanceObject, AuditVerb: settingsAuditVerb,
	MachineryApplied: true, // the nightly check reads it; no request carries it
})

// settingsAuditVerb is the audit action every deals settings change records.
const settingsAuditVerb = "update"

// SetLegalReviewEnabled switches legal review for the installation. An admin's
// act alone: ops holds installation_settings.update for the wiring it runs,
// and whether deals wait for legal is not wiring.
func SetLegalReviewEnabled(ctx context.Context, cfg *config.Config, on bool) error {
	if err := auth.RequireAdmin(ctx); err != nil {
		return err
	}
	return config.Save(ctx, cfg, LegalReviewEnabled, on)
}

// ConfigSettings are this module's settings in the configuration registry.
func ConfigSettings() []config.Any {
	return []config.Any{StageAutopilotEnabled, MaintenanceWritesEnabled, LegalReviewEnabled, SilentCloseDays}
}
