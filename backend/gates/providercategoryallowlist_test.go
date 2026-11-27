// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The allow-list of provider-defined containers lives in two places (the CHECK
// on capture_exclusion and the Go the writer consults), and the two must agree.
// The drift is silent and asymmetric: a token in the CHECK but not in Go is a
// rule the database would accept and the writer refuses; one in Go but not the
// CHECK is a write that passes validation and dies on a constraint violation,
// which reaches a reader as a 500.
//
// Both sides are read from source. A gate may not import a module (ADR-0054),
// and parsing is the right shape anyway. A reviewer checks the schema and the
// declaration, not a value assembled at runtime.

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// The constraint's list and the module's list name the same tokens.
func TestTheProviderContainerAllowListIsOneSet(t *testing.T) {
	t.Parallel()

	inCheck := providerContainersInTheConstraint(t)
	inGo := providerContainersInTheModule(t)
	sort.Strings(inCheck)
	sort.Strings(inGo)

	if len(inCheck) == 0 {
		t.Fatal("no provider-defined containers found in the constraint — the migration this reads moved or was rewritten")
	}
	if len(inGo) == 0 {
		t.Fatal("no provider-defined containers found in capture — the declaration this reads moved or was rewritten")
	}
	if strings.Join(inCheck, ",") != strings.Join(inGo, ",") {
		t.Fatalf("the allow-lists disagree:\n  constraint: %v\n  capture:    %v\n"+
			"A token in one and not the other is either a rule the writer refuses and the database accepts, "+
			"or a write that passes validation and dies on the constraint as a 500.", inCheck, inGo)
	}
}

// providerContainerArm is the allow-list arm of the constraint, as the head
// catalog spells it. The arm is one of three ORed terms, so the constraint is
// no whole vocabulary and catalogVocabularies does not hold it.
var providerContainerArm = regexp.MustCompile(
	`capture_exclusion_container_is_personal CHECK .*value = ANY \(ARRAY\[([^\]]*)\]`)

// providerContainersInTheConstraint reads the tokens out of the head catalog.
// A later migration that changes the constraint updates it, and the migration
// that first stated it would read stale.
func providerContainersInTheConstraint(t *testing.T) []string {
	t.Helper()
	arm := providerContainerArm.FindStringSubmatch(headCatalog(t))
	if arm == nil {
		t.Fatalf("%s no longer states capture_exclusion_container_is_personal with a value = ANY arm", headCatalogPath)
	}
	return splitCatalogTokens(arm[1])
}

// providerContainersInTheModule reads them out of the slice capture declares.
//
// The slice rather than the consts: what the writer consults is the slice, so
// a const declared and left out of it would be a token nothing enforces.
func providerContainersInTheModule(t *testing.T) []string {
	t.Helper()
	body := newestFileMatching(t, "internal/modules/capture/providercategories.go")
	decl := regexp.MustCompile(`(?s)providerDefinedContainers = \[\]string\{(.*?)\n\}`).FindSubmatch(body)
	if decl == nil {
		t.Fatal("capture no longer declares providerDefinedContainers as a []string literal")
	}
	// The slice names consts, so resolve each to the literal it is bound to.
	var out []string
	for _, name := range regexp.MustCompile(`(?m)^\s*(Gmail\w+),`).FindAllSubmatch(decl[1], -1) {
		bound := regexp.MustCompile(string(name[1]) + `\s*=\s*"([^"]+)"`).FindSubmatch(body)
		if bound == nil {
			t.Fatalf("%s is in the slice and bound to no literal", name[1])
		}
		out = append(out, string(bound[1]))
	}
	return out
}

func newestFileMatching(t *testing.T, pattern string) []byte {
	t.Helper()
	matches, err := gatekit.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("nothing matched %s: %v", pattern, err)
	}
	sort.Strings(matches)
	body, err := gatekit.ReadFile(matches[len(matches)-1])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[len(matches)-1], err)
	}
	return body
}
