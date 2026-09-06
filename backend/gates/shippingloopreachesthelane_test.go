// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// The routine the rulebook tells a contributor to run reaches the integration
// lane.
//
// It did not, and nothing said so. The shipping loop's step 2 named `make check`
// and added "with nothing to add on top" — while `check`'s test target is
// `go test ./...` and every integration file carries `//go:build integration`,
// so those packages compile into nothing. A contributor followed the documented
// routine exactly and pushed backend changes no local run had exercised. The
// lane's own security cases are in there.
//
// Two claims, and either alone rots into the same silence: the rulebook must
// name a target that reaches the lane, and that target must still reach it. A
// gate on the prose alone passes over a target that stopped running the lane;
// one on the Makefile alone passes over a rulebook that stopped naming it.

import (
	"path/filepath"
	"strings"
	"testing"
)

const integrationRoutine = "check-all"

func TestTheShippingLoopNamesARoutineThatReachesTheIntegrationLane(t *testing.T) {
	t.Parallel()

	rules := readRepoFile(t, filepath.Join(repoRoot, "AGENTS.md"))
	shipping := section(t, rules, "## Shipping a change")
	// The COMMAND, not the word: a step that mentions check-all while telling a
	// contributor to run something else reads to this gate exactly like one that
	// tells them to run it.
	if !strings.Contains(shipping, "`make "+integrationRoutine+"`") {
		t.Errorf("the shipping loop does not name `make %s`, so the routine a contributor is told "+
			"to run stops at `check` — which reaches the integration lane not at all, because its "+
			"test target is `go test ./...` and every integration file is behind a build tag",
			integrationRoutine)
	}

	root := readRepoFile(t, filepath.Join(repoRoot, "Makefile"))
	recipe := section(t, root, "\n"+integrationRoutine+":")
	if !invokes(recipe, "test-integration") {
		t.Errorf("`make %s` no longer runs test-integration, so the rulebook now points at a "+
			"routine as blind as the one it replaced — and blind in the way that reports success",
			integrationRoutine)
	}
}

// invokes reports whether a recipe RUNS a target rather than merely mentioning
// it. The recipe carries the reasoning for what it does and does not run, so
// the lane's name appears in its comments whatever the commands do — and a
// recipe that stopped running the lane would keep every one of those words.
func invokes(recipe, target string) bool {
	for _, line := range strings.Split(recipe, "\n") {
		command := strings.TrimSpace(line)
		if strings.HasPrefix(command, "#") {
			continue
		}
		if strings.Contains(command, "$(MAKE)") && strings.Contains(command, target) {
			return true
		}
	}
	return false
}

// section is the text from a heading or a recipe's first line to the start of
// the next one — a markdown "## " heading, or a Makefile recipe's unindented
// target line. Enough to read one step or one recipe whole without parsing
// either format, and it must be WHOLE: a reader that stopped at the first blank
// line would miss a numbered step two paragraphs down and report it missing.
func section(t *testing.T, text, start string) string {
	t.Helper()
	at := strings.Index(text, start)
	if at < 0 {
		t.Fatalf("%q is no longer in the file — this gate is reading a shape that is gone, which "+
			"is how a census comes to certify nothing", start)
	}
	rest := text[at+len(start):]
	for _, boundary := range []string{"\n## ", "\n\n\n"} {
		if end := strings.Index(rest, boundary); end >= 0 {
			rest = rest[:end]
		}
	}
	return rest
}
