// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// The ai module's own settings declarations (ADR-0090/A135).
//
// Routing is where the tier→model binding LIVES, moving it off a file read
// once at boot. The reason is not tidiness: a binding an operator can only
// change by editing a file on the server and restarting both roles is a
// binding they change rarely and out of band, and the two roles can disagree
// about it in between. In the database there is one answer, and it is the same
// answer for every role that asks.
//
// It is a setting rather than deployment config because re-pointing a tier is
// the admin's decision — it trades cost against quality for their own
// installation — and none of it is destructive or arms a capability. That is
// the line ADR-0061 §2 draws and settingscatalog_test.go enforces.

import (
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/settings"
)

// routingSettingsObject is the RBAC object gating the routing binding.
//
// Its own object rather than a shared ai one: what this governs is which
// vendor an installation's text is sent to, which is a different decision from
// reading a model's price, and a role that may see the second has no business
// changing the first.
const routingSettingsObject = "ai_routing"

// settingsAuditVerb is the audit action every ai settings change records.
const settingsAuditVerb = "update"

// RoutingKey is the settings key the binding is stored under, exported so the
// composition root can seed it without re-spelling the string.
const RoutingKey = "ai.routing"

// Routing is the deployment's tier→model binding (ai-operational-spec §1.4).
//
// The default is the ZERO config, which is "unconfigured" and not a fallback
// binding: an installation that has bound no models runs with its AI lanes
// absent, exactly as one with no routing file did. A default that named
// vendors would send an installation's text somewhere nobody chose.
//
// It SURVIVES a data reset: bootstrap takes it from the deployment
// configuration, like the installation's name. A reset wipes data, not the
// choice of vendor. Wiping it would also be quiet. A dev stack re-seeds the
// binding on the next boot; an installation with no seed loses its AI lanes.
//
// A boot value replaces the whole document. A binding is judged as one, and a
// tier from one source beside a profile from another is one nobody wrote.
var Routing = config.DefineStored(config.Spec[RoutingConfig]{
	Key: "ai.routing",
	// Not MARGINCE_AI_ROUTING: the retired --ai-routing flag keeps that name,
	// and an old routing-file path in it must not be read as a binding.
	Env:      "MARGINCE_AI_ROUTING_BINDING",
	Type:     config.Struct[RoutingConfig](),
	Default:  config.Some(RoutingConfig{}),
	Validate: validateStoredRouting,
	Doc:      "The tier-to-model binding, in the shape seeds.ai_routing takes (JSON in the environment). Unset binds no models.",
}, config.Store{RBACObject: routingSettingsObject, AuditVerb: settingsAuditVerb, SurvivesDataReset: true})

// ConfigSettings are the ai settings an administrator may also change at run
// time, apart from the provider keys, which Settings lists.
func ConfigSettings() []config.Any {
	return []config.Any{Routing, TaskOverridesSetting, BudgetSettings, PriceSyncSettings}
}

// Definitions are the ai rows that are not configuration: the record of the
// last price sync.
func Definitions() []settings.Definition {
	return []settings.Definition{PriceSyncLastRun}
}

// validateStoredRouting holds a stored binding to the same bar the file always
// had, so a write through the settings surface cannot land a config the file
// loader would have refused at boot. It finalizes first, because a stored
// document is canonical: its lanes read their host and pins from providers.
//
// A document that binds nothing is the one exception: it is how "no models are
// bound" is spelled and it is the registered default, which has to validate.
// Its provider entries, set ahead of any binding, are held to their shape. A
// document that sets a profile and binds no tiers is NOT that — the file loader
// refuses it ("no tiers bound"), and accepting it here would store something an
// operator wrote, that reads as configured, and that routes nothing.
func validateStoredRouting(cfg RoutingConfig) error {
	if cfg.zero() {
		return cfg.validateProviderEntries()
	}
	_, err := cfg.finalize()
	return err
}

// Unconfigured reports whether this config binds nothing at all — the state an
// installation is in before anyone has chosen models, and the one the AI lanes
// stay absent for.
//
// Tiers is what decides it: a config carrying a profile and no tiers routes
// nothing, and treating that as configured would build a Router that refuses
// every call it is handed. This is the RUNTIME question — "is there anything to
// serve" — which is a different question from whether a document may be stored;
// see zero.
func (cfg RoutingConfig) Unconfigured() bool { return len(cfg.Tiers) == 0 }

// zero reports whether no binding was set, which is the only document the
// validator exempts from the file loader's bar. Providers do not count: an
// entry no lane binds routes nothing, and the validator still checks its shape.
//
// Deliberately narrower than Unconfigured. Both mean "serves nothing", but a
// document that names a profile is one somebody WROTE, and storing it
// unexamined would accept a half-bound binding that reads as configured and
// routes nothing.
func (cfg RoutingConfig) zero() bool {
	e := cfg.Embeddings
	return cfg.Profile == "" && len(cfg.Tiers) == 0 && cfg.Decisions == nil &&
		e.Provider == "" && e.Model == "" && e.BaseURL == "" && len(e.Input) == 0 && e.Dimensions == 0
}
