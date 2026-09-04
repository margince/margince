// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// How much authority the authorization engine's answer carries, per category.
//
// The engine and the old purpose gate both answer every outbound send, and
// this is what decides which one rules while the two are compared. It is per
// category because the categories become trustworthy at different times: a
// reply's evidence is a thread the subject started and is settled today, while
// marketing waits for the jurisdiction packs — enforcing it before the German
// existing-customer exception exists would refuse sends that are lawful under
// §7(3), and the engine would be stricter than the law.
//
// It is NOT a way to switch the engine off. Four refusals bind in every mode —
// an Art. 21 objection, a processing restriction, a hard bounce and an
// unconfirmed double opt-in, plus a recipient nobody could resolve and a
// withdrawn consent. Those are decided by law or by the subject, not by how
// far along a rollout is (commsauthz/absolute.go holds the list). Rolling a
// category back to observe re-admits the old gate's answer; it never
// resurrects a consent, clears a suppression or reaches past those six.

import (
	"fmt"
	"sort"
	"strings"

	"github.com/margince/margince/backend/internal/platform/settings"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// authorizationModesObject gates the rollout posture. installation_settings
// rather than a new object: this is one installation-wide answer about how the
// product behaves, the same shape as every other posture living there, and a
// new RBAC object would need a presence-guarded backfill for every workspace
// created before it existed.
const authorizationModesObject = "installation_settings"

// AuthorizationModes maps a communication category to the authority the
// engine's answer carries for it.
//
// A category absent from the map is `observe`, so a category added to the
// vocabulary tomorrow arrives in the position where it is recorded and not yet
// binding, whatever the stored map says. That default is what makes the map a
// set of EXCEPTIONS rather than a table that must be complete to be correct.
//
// The shipped default names every category that exists today, at `enforce`.
// Observe was the rollout, not the destination: it ran so the disagreement
// between the engine and the old purpose gate could be read before it bound
// anything, and the reading came back empty — where the engine has no evidence
// of its own it adopts the old gate's answer rather than refusing, so enforcing
// stops nothing that was being sent. What it ENDS is the old gate overruling
// the engine on a message the engine can evidence and it cannot.
var AuthorizationModes = settings.Define[map[string]string](
	"consent.authorization_modes",
	authorizationModesObject,
	"update",
	enforceEveryCategory(),
	validateAuthorizationModes,
	// MachineryApplied: the transmit gate reads this inside the transaction
	// that binds its own decision, and the posture must apply whoever the
	// acting principal is — a worker dispatching a delivery holds the system
	// principal and has no settings read gate to pass.
).MachineryApplied()

// enforceEveryCategory is the shipped posture: every category the vocabulary
// holds, at enforce.
//
// DERIVED from the vocabulary rather than typed out, because a hand-written map
// is a second list that drifts. A category added to commsauthz and forgotten
// here would silently ship observing — recorded, not binding — which is exactly
// the state this default exists to leave behind, and nothing would say so.
func enforceEveryCategory() map[string]string {
	out := make(map[string]string, len(commsauthz.Categories()))
	for _, c := range commsauthz.Categories() {
		out[string(c)] = string(commsauthz.ModeEnforce)
	}
	return out
}

// validateAuthorizationModes refuses a map that names something that is not a
// category, or a mode that is not a mode.
//
// Both halves are checked against the vocabularies rather than against a list
// here: a typo in a category key would otherwise be accepted and then silently
// mean nothing, which reads exactly like a rollout that was configured and did
// not take.
func validateAuthorizationModes(in map[string]string) error {
	var unknownCategories, unknownModes []string
	for category, mode := range in {
		if !commsauthz.Category(category).Valid() {
			unknownCategories = append(unknownCategories, category)
		}
		switch commsauthz.Mode(mode) {
		case commsauthz.ModeObserve, commsauthz.ModeWarn, commsauthz.ModeEnforce:
		default:
			unknownModes = append(unknownModes, mode)
		}
	}
	sort.Strings(unknownCategories)
	sort.Strings(unknownModes)
	if len(unknownCategories) > 0 {
		return fmt.Errorf("not a communication category: %s", strings.Join(unknownCategories, ", "))
	}
	if len(unknownModes) > 0 {
		return fmt.Errorf("a mode is observe, warn or enforce, not: %s", strings.Join(unknownModes, ", "))
	}
	return nil
}

// ModeFor answers the authority the engine carries for one category.
//
// Absent means observe, which is what makes the stored map a set of
// EXCEPTIONS to the safe default rather than a table that must list every
// category to be correct. A map that had to be complete would put a new
// category into whatever position the last author typed.
func ModeFor(modes map[string]string, category commsauthz.Category) commsauthz.Mode {
	switch commsauthz.Mode(modes[string(category)]) {
	case commsauthz.ModeEnforce:
		return commsauthz.ModeEnforce
	case commsauthz.ModeWarn:
		return commsauthz.ModeWarn
	default:
		return commsauthz.ModeObserve
	}
}

// Definitions is consent's contribution to the settings registry; compose
// concatenates each module's list.
func Definitions() []settings.Definition {
	return []settings.Definition{AuthorizationModes}
}
