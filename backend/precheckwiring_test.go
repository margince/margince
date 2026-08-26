// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// A precheck that exists but is not wired protects nothing.
//
// approvals.Service.WithPrecheck refuses a DECISION whose payload the effect
// could not use, which is the only defence a kind has against an approved row
// nothing can re-decide: the decision commits before the effect runs, and a
// failed effect never un-decides it. Registering it is one line in the
// composition, and forgetting that line is invisible — every test that wires
// its own service still passes, and the product silently records a yes that
// produces nothing.
//
// So the obligation is derived from the source rather than listed: a
// `func xPrecheck(` under internal/compose must appear in a WithPrecheck call
// somewhere under internal/compose that is not a test file. Writing one and not
// wiring it is the failure this catches.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// A precheck reaches WithPrecheck two ways in this tree: passed directly, or
// carried as a struct field through a registration table (sendpath.go's
// lateApprovalEffects). Matching only the direct call reported that table's
// entry as unwired, so what counts as wiring is any REFERENCE to the function
// from a non-test file other than its own declaration — a name mentioned
// nowhere else is the shape this gate is actually looking for.
var (
	precheckDecl = regexp.MustCompile(`(?m)^func ([A-Za-z0-9_]*[Pp]recheck)\(`)
	precheckRef  = regexp.MustCompile(`\b([A-Za-z0-9_]*[Pp]recheck)\b`)
)

func TestEveryPrecheckInComposeIsWired(t *testing.T) {
	declared := map[string]string{}
	// Every non-test file each name appears in, INCLUDING the one that declares
	// it. A name mentioned only inside its own file is unwired — the first cut
	// of this gate counted that file's own doc comments as a use, which made it
	// pass while the production line was deleted.
	refs := map[string][]string{}

	root := filepath.Join("internal", "compose")
	if err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		body, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(body)
		isTest := strings.HasSuffix(path, "_test.go")
		for _, m := range precheckDecl.FindAllStringSubmatch(text, -1) {
			if !isTest {
				declared[m[1]] = path
			}
		}
		// A test file wiring its own service proves nothing about production,
		// which is exactly the hole here: the harness in
		// reconcile_integration_test.go registers the precheck itself, so
		// dropping the production line leaves every test green.
		if !isTest {
			for _, m := range precheckRef.FindAllStringSubmatch(text, -1) {
				refs[m[1]] = append(refs[m[1]], path)
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("walking %s: %v", root, err)
	}

	if len(declared) == 0 {
		t.Fatal("no precheck found under internal/compose, so this gate read a smaller tree than it thinks")
	}
	for name, path := range declared {
		var elsewhere bool
		for _, seen := range refs[name] {
			if seen != path {
				elsewhere = true
				break
			}
		}
		if !elsewhere {
			t.Errorf("%s declares %s but no non-test file under internal/compose calls "+
				"WithPrecheck with it — an unwired precheck refuses nothing, and the "+
				"kind's effect can still fail after its decision has committed", path, name)
		}
	}
}
