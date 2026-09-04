// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What the rollout posture accepts, and what it means when it says nothing.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// Absent means observe, which is what lets the stored map be a set of
// EXCEPTIONS rather than a table that must list every category to be correct.
// A map that had to be complete would put a category added tomorrow into
// whatever position the last author happened to type.
func TestACategoryTheMapDoesNotNameObserves(t *testing.T) {
	modes := map[string]string{string(commsauthz.CategoryReplyToInbound): "enforce"}
	if got := ModeFor(modes, commsauthz.CategoryMarketing); got != commsauthz.ModeObserve {
		t.Errorf("an unnamed category resolved to %q, want observe", got)
	}
	if got := ModeFor(modes, commsauthz.CategoryReplyToInbound); got != commsauthz.ModeEnforce {
		t.Errorf("a named category resolved to %q, want enforce", got)
	}
}

// Every category in the vocabulary observes under the shipped default. This is
// derived from the vocabulary rather than listed, so a category added without a
// thought about its rollout position still arrives in the safest one.
func TestTheDefaultObservesEveryCategory(t *testing.T) {
	def := map[string]string{}
	for _, c := range commsauthz.Categories() {
		if got := ModeFor(def, c); got != commsauthz.ModeObserve {
			t.Errorf("%s defaults to %q, want observe", c, got)
		}
	}
}

// A value that is not a mode resolves to observe rather than to itself. A
// stored map can predate a validator, and a garbled value must fail safe.
func TestAnUnreadableModeObserves(t *testing.T) {
	modes := map[string]string{string(commsauthz.CategoryMarketing): "enforced"}
	if got := ModeFor(modes, commsauthz.CategoryMarketing); got != commsauthz.ModeObserve {
		t.Errorf("a garbled mode resolved to %q, want observe", got)
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
func TestTheEmptyMapValidates(t *testing.T) {
	if err := validateAuthorizationModes(map[string]string{}); err != nil {
		t.Errorf("the default posture was refused: %v", err)
	}
}
