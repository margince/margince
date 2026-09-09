// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the rollout posture accepts, and what it means when it says nothing.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Absent means ENFORCE, and the validator refuses a partial map, so the two
// together mean no category reaches the engine without a decision about it.
// Absent used to mean observe: a category nobody remembered to name then
// shipped recorded-but-not-binding, with nothing saying so.
func TestACategoryTheMapDoesNotNameEnforces(t *testing.T) {
	modes := map[string]string{string(commsauthz.CategoryReplyToInbound): "observe"}
	if got := ModeFor(modes, commsauthz.CategoryMarketing); got != commsauthz.ModeEnforce {
		t.Errorf("an unnamed category resolved to %q, want enforce", got)
	}
	if got := ModeFor(modes, commsauthz.CategoryReplyToInbound); got != commsauthz.ModeObserve {
		t.Errorf("a named category resolved to %q, want observe", got)
	}
}

// Every category in the vocabulary enforces under the shipped default. Derived
// from the vocabulary rather than listed, so a category added without a thought
// about its rollout position arrives bound by the engine's own answer rather
// than deferring to the legacy gate.
func TestTheDefaultEnforcesEveryCategory(t *testing.T) {
	def := map[string]string{}
	for _, c := range commsauthz.Categories() {
		if got := ModeFor(def, c); got != commsauthz.ModeEnforce {
			t.Errorf("%s defaults to %q, want enforce", c, got)
		}
	}
}

// A value that is not a mode resolves to enforce rather than to itself. A
// stored map can predate a validator, and failing safe now means binding the
// engine's own answer rather than handing the message back to the old gate.
func TestAnUnreadableModeEnforces(t *testing.T) {
	modes := map[string]string{string(commsauthz.CategoryMarketing): "enforced"}
	if got := ModeFor(modes, commsauthz.CategoryMarketing); got != commsauthz.ModeEnforce {
		t.Errorf("a garbled mode resolved to %q, want enforce", got)
	}
}

// A map naming some categories and not others is refused, naming the ones with
// no mode. This is the other half of absent-means-enforce: the default is safe,
// and a half-written map is a question that gets asked rather than answered.
func TestAPartialMapIsRefused(t *testing.T) {
	err := validateAuthorizationModes(map[string]string{
		string(commsauthz.CategoryMarketing): "enforce",
	})
	if err == nil {
		t.Fatal("a map naming one category of fourteen was accepted")
	}
	if !strings.Contains(err.Error(), string(commsauthz.CategoryReplyToInbound)) {
		t.Errorf("the refusal does not name a category that has no mode: %v", err)
	}
}

// A misspelled category is refused rather than stored. Accepting one would let
// it silently mean nothing, which reads exactly like a rollout that was
// configured and did not take.
func TestAnUnknownCategoryIsRefused(t *testing.T) {
	err := validateAuthorizationModes(map[string]string{"transactional": "enforce"})
	if err == nil {
		t.Fatal("a category that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "transactional") {
		t.Errorf("the refusal does not name what was wrong: %v", err)
	}
}

// And a mode that is not a mode.
func TestAnUnknownModeIsRefused(t *testing.T) {
	err := validateAuthorizationModes(map[string]string{
		string(commsauthz.CategoryMarketing): "block",
	})
	if err == nil {
		t.Fatal("a mode that does not exist was accepted")
	}
	if !strings.Contains(err.Error(), "block") {
		t.Errorf("the refusal does not name what was wrong: %v", err)
	}
}

// Every real category paired with every real mode is accepted, so the
// validator above refuses the wrong thing and not the right one.
func TestEveryCategoryAndModeIsAccepted(t *testing.T) {
	for _, mode := range []commsauthz.Mode{
		commsauthz.ModeObserve, commsauthz.ModeWarn, commsauthz.ModeEnforce,
	} {
		in := map[string]string{}
		for _, c := range commsauthz.Categories() {
			in[string(c)] = string(mode)
		}
		if err := validateAuthorizationModes(in); err != nil {
			t.Errorf("every category in %s was refused: %v", mode, err)
		}
	}
}

// An empty map is the shipped default and must validate, or no installation
// could save any other setting on the same surface.
//
// This is the one case TestAPartialMapIsRefused deliberately does not cover.
// Empty means unconfigured and resolves to enforce through ModeFor; a map
// naming SOME categories means somebody made per-category decisions, and the
// omissions there are the ones they did not think about.
func TestTheEmptyMapValidates(t *testing.T) {
	if err := validateAuthorizationModes(map[string]string{}); err != nil {
		t.Errorf("the default posture was refused: %v", err)
	}
}
