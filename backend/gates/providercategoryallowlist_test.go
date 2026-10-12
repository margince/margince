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
// Both sides are read from SOURCE. A gate may not import a module (ADR-0054
// §3), and parsing is the right shape anyway: what a reviewer checks is the
// migration and the declaration, not a value assembled at runtime.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
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

// providerContainersInTheConstraint reads the tokens out of the migration that
// states the allow-list.
//
// Only the quoted tokens inside the `value IN (...)` arm: the surrounding
// prose names them too, and a comment must not be able to satisfy a gate.
func providerContainersInTheConstraint(t *testing.T) []string {
	t.Helper()
	body := newestFileMatching(t, "migrations/core/*_a_provider_category_binds_everybody.up.sql")
	arm := regexp.MustCompile(`(?s)value IN \((.*?)\)`).FindSubmatch(body)
	if arm == nil {
		t.Fatal("the migration no longer states the allow-list as a value IN (...) arm")
	}
	return quotedTokens(arm[1])
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
	matches, err := filepath.Glob(pattern)
	if err != nil || len(matches) == 0 {
		t.Fatalf("nothing matched %s: %v", pattern, err)
	}
	sort.Strings(matches)
	body, err := os.ReadFile(matches[len(matches)-1])
	if err != nil {
		t.Fatalf("reading %s: %v", matches[len(matches)-1], err)
	}
	return body
}

func quotedTokens(in []byte) []string {
	var out []string
	for _, m := range regexp.MustCompile(`'([^']+)'`).FindAllSubmatch(in, -1) {
		out = append(out, string(m[1]))
	}
	return out
}
