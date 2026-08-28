// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates_test

// The signing scope is ONE invariant spelled on both sides of a wire.
//
// The verifier reads it from extension.ScopeInbound; the member reads it from a
// `curl` the connector's screen generates and pastes into a terminal. Neither
// side can see the other, and the failure when they disagree is the worst shape
// available at this edge: every refusal here is one opaque 401 by design, so a
// member whose command silently stopped verifying gets an answer with nothing
// in it to say why — not a wrong scope, not a wrong signature, nothing. They
// would blame the secret, mint a new one, and get the same 401 again.
//
// Held in BOTH directions, because either half moving alone breaks it: a scope
// bumped in Go leaves every published recipe minting signatures nothing
// accepts, and a scope edited in the screen leaves a command that never worked
// in the one place a member is told to copy from.
//
// Ordinary version bumps are expected — that is what a scope is for. This gate
// does not refuse one; it refuses HALF of one.

import (
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// recipeFile is the screen the member copies the command from.
const recipeFile = "../extensions/openchannel/frontend/recipe.tsx"

// scopeInFrontend reads the exported constant rather than any of the prose
// around it, so a comment that mentions the old value does not hold the gate
// green after the code has moved on.
var scopeInFrontend = regexp.MustCompile(`export const SCOPE_INBOUND = "([^"]+)"`)

func TestTheSigningScopeIsSpelledTheSameOnBothSidesOfTheWire(t *testing.T) {
	source, err := os.ReadFile(recipeFile)
	if err != nil {
		t.Fatalf("reading %s: %v", recipeFile, err)
	}
	match := scopeInFrontend.FindSubmatch(source)
	if match == nil {
		t.Fatalf("%s no longer exports SCOPE_INBOUND as a string literal — this gate reads that "+
			"shape, so a different one leaves the two sides of the signature unheld", recipeFile)
	}
	if got, want := string(match[1]), string(extension.ScopeInbound); got != want {
		t.Errorf("the screen signs under scope %q and the verifier accepts %q — the command a member "+
			"is told to paste mints a signature this installation refuses, and the refusal is the same "+
			"opaque 401 as a wrong secret, so nothing in the answer says which", got, want)
	}
}

// The scope must also actually REACH the generated command. A constant the file
// exports but no longer interpolates would pass the comparison above while the
// recipe signed something else entirely.
func TestTheGeneratedCommandSignsUnderTheScope(t *testing.T) {
	source, err := os.ReadFile(recipeFile)
	if err != nil {
		t.Fatalf("reading %s: %v", recipeFile, err)
	}
	for _, needed := range []string{"SCOPE_LITERAL", "${SCOPE_INBOUND}"} {
		if !regexp.MustCompile(regexp.QuoteMeta(needed)).Match(source) {
			t.Errorf("%s exports the scope but its command no longer carries %s — the recipe signs "+
				"material the verifier does not expect", recipeFile, needed)
		}
	}
}
