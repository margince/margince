// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deals module's own settings declarations.
//
// Both entries are kill switches rather than preferences: the installation's
// answer to whether stage automation may move a deal, and whether the nightly
// maintenance sweep may write to one.

import (
	"github.com/margince/margince/backend/internal/platform/settings"
)

// StageAutopilotEnabled is the installation-wide answer to whether a stage
// transition may ever move a deal without asking.
//
// DEFAULT FALSE, and that is the whole point of having it. Every other control
// on this feature is per-transition and earned: a pipeline's rule turns on only
// once the measured rates clear their thresholds. This one is neither earned
// nor per-transition — it is the switch an admin reaches for when something is
// wrong and they do not yet know which transition caused it, and a switch that
// defaulted to on would have to be found before it could be used.
//
// It is read INSIDE the transaction that decides an automatic apply, alongside
// the pipeline's rule and the rates, so flipping it off stops the next apply
// rather than the one after. Read at staging instead, it would authorize a
// move hours before that move happened.
var StageAutopilotEnabled = settings.Define[bool](
	"deals.stage_autopilot_enabled",
	stageAutomationObject,
	"update",
	false,
	nil, // a bool has two values and both mean something; nothing to validate
).MachineryApplied() // the apply path reads it; no request carries it

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
var MaintenanceWritesEnabled = settings.Define[bool](
	"deals.maintenance_writes_enabled",
	maintenanceObject,
	"update",
	true,
	nil, // a bool has two values and both mean something; nothing to validate
).MachineryApplied() // the sweep reads it; no request carries it

// maintenanceObject is the RBAC object gating maintenance governance. `deal`,
// because a deal is what the sweep writes — the grant that lets somebody update
// a deal is the one that lets them say whether the machine may.
const maintenanceObject = "deal"

// Definitions is the deals module's contribution to the settings catalog.
func Definitions() []settings.Definition {
	return []settings.Definition{StageAutopilotEnabled, MaintenanceWritesEnabled}
}
