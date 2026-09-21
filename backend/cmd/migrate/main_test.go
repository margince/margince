// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

// The extension lane's pure half: turning the composed set into migration
// namespaces, and saying out loud which ones were found. Both are the
// difference between a migrate that applies an installation's extension
// schema and one that silently applies none, so neither is left to the
// integration lane alone.

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

func TestReportExtensionNamespacesSaysSoWhenThereAreNone(t *testing.T) {
	var out strings.Builder
	if err := reportExtensionNamespaces(nil, &out); err != nil {
		t.Fatalf("reportExtensionNamespaces: %v", err)
	}
	// Silence here is the failure this wiring exists to prevent: a migrate
	// resolving the vanilla stub applies zero extension migrations and would
	// otherwise look exactly like a correct run.
	if !strings.Contains(out.String(), "none in the composed set") {
		t.Errorf("empty set printed %q, want an explicit line saying none were composed", out.String())
	}
}

func TestReportExtensionNamespacesNamesEachLaneAndItsSize(t *testing.T) {
	var out strings.Builder
	err := reportExtensionNamespaces([]dbmigrate.Namespace{
		{Name: "ext_alpha", Migrations: []dbmigrate.Migration{{Version: "0001"}}},
		{Name: "ext_zulu", Migrations: []dbmigrate.Migration{{Version: "0001"}, {Version: "0002"}}},
	}, &out)
	if err != nil {
		t.Fatalf("reportExtensionNamespaces: %v", err)
	}
	got := out.String()
	for _, want := range []string{"ext_alpha (1 declared)", "ext_zulu (2 declared)"} {
		if !strings.Contains(got, want) {
			t.Errorf("output %q is missing %q", got, want)
		}
	}
}

// failWriter is a stdout that cannot be written to — a closed pipe, say.
type failWriter struct{}

var errWriteFailed = errors.New("write failed")

func (failWriter) Write([]byte) (int, error) { return 0, errWriteFailed }

func TestReportExtensionNamespacesPropagatesAWriteFailure(t *testing.T) {
	for _, tc := range []struct {
		name string
		exts []dbmigrate.Namespace
	}{
		{"empty set", nil},
		{"populated set", []dbmigrate.Namespace{{Name: "ext_alpha"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// A migrate whose log went nowhere must not report success: the
			// line IS the operator's evidence the lane ran.
			if err := reportExtensionNamespaces(tc.exts, failWriter{}); !errors.Is(err, errWriteFailed) {
				t.Errorf("err = %v, want it to wrap the write failure", err)
			}
		})
	}
}

// shellMatcherPattern finds migrate_template's comparison in
// scripts/lib-testdb.sh and captures the literal prefix it tests the summary
// against.
var shellMatcherPattern = regexp.MustCompile(`\[\[ "\$summary" != "([^"]*)"\* \]\]`)

// entrypointProvisionedPattern finds the deploy entrypoint's comparison against
// the workspace-exists answer and captures the literal it expects.
var entrypointProvisionedPattern = regexp.MustCompile(`\[ "\$provisioned" = "([^"]*)" \]`)

// TestWorkspaceExistsAnswerMatchesTheEntrypointComparison pins the other half of a
// wire contract that is otherwise only exercised in a container nothing in CI
// runs: the entrypoint string-compares this verb's stdout to decide whether to
// write a plaintext bootstrap credential. Drift in either direction is silent
// and lands on the wrong side of that decision — print "TRUE" and every
// provisioned installation gets the credential written again.
func TestWorkspaceExistsAnswerMatchesTheEntrypointComparison(t *testing.T) {
	const script = "../../../scripts/deploy/api-entrypoint.sh"
	source, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("reading %s: %v", script, err)
	}
	found := entrypointProvisionedPattern.FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s no longer compares $provisioned against a literal — the bootstrap-credential branch was rewritten; re-point this test at whatever replaced it", script)
	}
	want := string(found[1])

	var out bytes.Buffer
	// The exact call workspaceExists makes to report a provisioned installation. The
	// shell's $(…) strips the trailing newline, so the comparison is against the
	// trimmed form.
	if _, err := fmt.Fprintf(&out, "%t\n", true); err != nil {
		t.Fatalf("rendering the answer: %v", err)
	}
	if got := strings.TrimSpace(out.String()); got != want {
		t.Errorf("workspace-exists prints %q for a provisioned installation but %s branches on %q — the entrypoint would write a plaintext bootstrap credential onto a live installation; change both together", got, script, want)
	}
}

// TestUpSummaryMatchesTheShellMatcher closes a silent-drift risk between two
// files that cannot see each other: cmd/migrate prints the summary and
// scripts/lib-testdb.sh string-matches it.
//
// A mismatch is not loud. migrate_template would print "was behind" on every
// run — a staleness check that cries wolf permanently is worse than none, and
// build_template discards its output, so the warning would not even be seen
// where it is most likely to be produced. Both sides are read here rather
// than restated, so this test cannot itself go stale.
func TestUpSummaryMatchesTheShellMatcher(t *testing.T) {
	const script = "../../../scripts/lib-testdb.sh"
	source, err := os.ReadFile(script)
	if err != nil {
		t.Fatalf("reading %s: %v", script, err)
	}
	found := shellMatcherPattern.FindSubmatch(source)
	if found == nil {
		t.Fatalf("%s no longer compares $summary against a literal prefix — migrate_template's staleness check was rewritten; re-point this test at whatever replaced it", script)
	}
	prefix := string(found[1])
	// The zero-applied form is the one the shell classifies on: it means
	// "nothing was missing", which is migrate_template's silent path.
	summary := fmt.Sprintf(upSummaryFormat, 0, 0)
	if !strings.HasPrefix(summary, prefix) {
		t.Errorf("migrate prints %q but %s matches on prefix %q — migrate_template would report every template as behind; change both together", summary, script, prefix)
	}
}
