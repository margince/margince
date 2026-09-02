// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The OAuth landing outcome is one vocabulary spelled on both sides of a
// redirect: the api puts it in the URL the provider sends a human back to, and
// the SPA turns it into the sentence that human reads.
//
// A value only the server knows renders NOTHING — the maps ignore an unknown
// segment rather than print it — so the failure is a person staring at a blank
// card after a connection did not work, with the reason sitting in a log they
// cannot see. A value only the frontend knows is the opposite tell: copy that
// can never appear, which is how a reader learns the map is not maintained.
//
// Both directions, because this vocabulary has already drifted once in the
// direction a one-ended check cannot see: `misconfigured` carried two faults
// with different remedies, and the screen named the wrong one for every
// Microsoft installation while the log beside it named the right one.
//
// The corpus is PARSED from the owner rather than listed here. A list in this
// file would be a third copy, and the one that agrees with itself while both
// others move.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	outcomeOwner       = "internal/compose/connectors_outcome.go"
	outcomeSettingsMap = "../frontend/src/screens/connectors.tsx"
	outcomePanelsMap   = "../frontend/src/screens/onboarding-connect-panels.tsx"
)

// outcomeConst matches the declarations that DEFINE the vocabulary.
var outcomeConst = regexp.MustCompile(`(?m)^\s*outcome[A-Za-z]+\s*=\s*"([a-z_]+)"`)

// settingsOutcomeKey matches one entry of the SPA's outcome→copy record.
var settingsOutcomeKey = regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*\{\s*key:\s*"connectors\.`)

func serverOutcomes(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(outcomeOwner)
	if err != nil {
		t.Fatalf("reading the outcome owner: %v", err)
	}
	var out []string
	for _, m := range outcomeConst.FindAllStringSubmatch(string(source), -1) {
		out = append(out, m[1])
	}
	// NOT a tolerated zero: the constants are there, so an empty read means the
	// pattern stopped seeing them and this gate now proves nothing.
	if len(out) < 3 {
		t.Fatalf("found %d outcome constant(s) in %s — the detection has gone blind", len(out), outcomeOwner)
	}
	slices.Sort(out)
	return slices.Compact(out)
}

// Every outcome the api can redirect with renders a sentence, and every
// sentence the settings screen holds is one the api can send.
func TestEveryOAuthOutcomeRendersAndEveryRenderedOneExists(t *testing.T) {
	t.Parallel()
	server := serverOutcomes(t)

	source, err := os.ReadFile(outcomeSettingsMap)
	if err != nil {
		t.Fatalf("reading the settings outcome map: %v", err)
	}
	var rendered []string
	for _, m := range settingsOutcomeKey.FindAllStringSubmatch(string(source), -1) {
		rendered = append(rendered, m[1])
	}
	if len(rendered) == 0 {
		t.Fatalf("found no outcome entries in %s — the detection has gone blind", outcomeSettingsMap)
	}

	for _, o := range server {
		if !slices.Contains(rendered, o) {
			t.Errorf("the api can land on %q and %s renders nothing for it — a human sees a blank card "+
				"after a connection failed, with the reason only in a log they cannot read", o, outcomeSettingsMap)
		}
	}
	for _, o := range rendered {
		if !slices.Contains(server, o) {
			t.Errorf("%s renders copy for %q, which the api never sends — dead copy is how the next "+
				"reader learns this map is not maintained", outcomeSettingsMap, o)
		}
	}
}

// The onboarding panels hold the SUBSET no retry can clear, and each entry has
// to be a real outcome. They point at the same copy Settings renders, so a
// reader gets one account of what happened wherever they were standing.
func TestTheOnboardingPanelsNameOnlyRealOutcomes(t *testing.T) {
	t.Parallel()
	server := serverOutcomes(t)

	source, err := os.ReadFile(outcomePanelsMap)
	if err != nil {
		t.Fatalf("reading the connect panels: %v", err)
	}
	block := string(source)
	start := strings.Index(block, "PERMANENT_FAILURE_BODY")
	if start < 0 {
		t.Fatalf("%s no longer declares PERMANENT_FAILURE_BODY — this gate reads nothing", outcomePanelsMap)
	}
	end := strings.Index(block[start:], "};")
	if end < 0 {
		t.Fatalf("could not find the end of PERMANENT_FAILURE_BODY in %s", outcomePanelsMap)
	}
	entry := regexp.MustCompile(`(?m)^\s*([a-z_]+):\s*"connectors\.`)
	found := entry.FindAllStringSubmatch(block[start:start+end], -1)
	if len(found) == 0 {
		t.Fatalf("found no entries in PERMANENT_FAILURE_BODY — the detection has gone blind")
	}
	for _, m := range found {
		if !slices.Contains(server, m[1]) {
			t.Errorf("the connect panels answer %q, which the api never sends", m[1])
		}
	}
}

// A problem CODE the api answers with is the other half of the same bargain:
// the SPA picks copy from it, and one it does not recognise falls through to
// the detail — which is written in one language and reaches a reader in
// whatever language they set.
//
// Scoped to httperr.Unavailable, the helper that EXISTS to carry a code for a
// reader. Every problem code in the tree is a wider question with real debt
// behind it (codes today that no catalog answers), and a gate that failed on
// all of it would be turned off rather than fixed. What this holds is the
// narrow promise the helper makes: if you reach for the one that takes a code,
// the client half is not optional.
const problemCodeMapping = "../frontend/src/screens/common.tsx"

var (
	unavailableCall = regexp.MustCompile(`httperr\.Unavailable\(\s*w,\s*r,\s*([A-Za-z][A-Za-z0-9_]*|"[a-z_]+")`)
	codeConstDecl   = regexp.MustCompile(`(?m)^\s*(?:const\s+)?([A-Za-z][A-Za-z0-9_]*)\s*=\s*"([a-z_]+)"`)
)

func TestEveryReaderFacingProblemCodeHasClientCopy(t *testing.T) {
	t.Parallel()

	// WALKED, not listed. Reading one directory would see the call that is
	// there today and miss the one added in a subpackage tomorrow — a census
	// that fails short reports PASS with nothing to notice, which is the one
	// way a gate must not break.
	//
	// Constants are collected PER PACKAGE. A flat map keyed on the identifier
	// resolves a call against whichever declaration the walk happened to visit
	// last, so two packages that spell one name differently would answer for
	// each other — and the direction that hurts is the silent one, where a code
	// with no copy borrows a mapped value from somewhere else and passes.
	consts := map[string]map[string]string{}
	type callSite struct{ pkg, ref string }
	var wanted []callSite
	if err := filepath.WalkDir("internal", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		source, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		pkg := filepath.Dir(path)
		text := string(source)
		for _, m := range codeConstDecl.FindAllStringSubmatch(text, -1) {
			if consts[pkg] == nil {
				consts[pkg] = map[string]string{}
			}
			consts[pkg][m[1]] = m[2]
		}
		for _, m := range unavailableCall.FindAllStringSubmatch(text, -1) {
			wanted = append(wanted, callSite{pkg: pkg, ref: m[1]})
		}
		return nil
	}); err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	// NOT a skip. compose reaches for this helper today, so an empty read means
	// the detection stopped seeing it — and a census that can fail short
	// reports PASS with nothing to notice.
	if len(wanted) == 0 {
		t.Fatal("no httperr.Unavailable call was found under internal/ — the detection has gone blind")
	}

	mapping, err := os.ReadFile(problemCodeMapping)
	if err != nil {
		t.Fatalf("reading the problem mapping: %v", err)
	}
	var codes []string
	for _, w := range wanted {
		code := strings.Trim(w.ref, `"`)
		if resolved, ok := consts[w.pkg][w.ref]; ok {
			code = resolved
		}
		codes = append(codes, code)
	}
	// Sorted before compacting: Compact drops only ADJACENT repeats, so a code
	// answered from two packages would be reported twice unsorted.
	slices.Sort(codes)
	for _, code := range slices.Compact(codes) {
		if !strings.Contains(string(mapping), `"`+code+`"`) {
			t.Errorf("the api answers problem code %q and %s maps no copy to it — the reader gets the "+
				"English detail whatever language they set", code, problemCodeMapping)
		}
	}
}
