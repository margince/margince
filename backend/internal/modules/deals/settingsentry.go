// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The deals module's own settings declarations.
//
// One entry today, and it is a kill switch rather than a preference: the
// installation's answer to whether stage automation may move a deal at all.

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

// Definitions is the deals module's contribution to the settings catalog.
func Definitions() []settings.Definition {
	return []settings.Definition{StageAutopilotEnabled}
}
