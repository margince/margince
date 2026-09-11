// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The scan's file-encoding obligation. SonarCloud's text-and-secrets sensor
// indexes EVERY file whatever its suffix, opens each as UTF-8, and warns on the
// ones that are not — and one such line is enough to leave the analysis
// carrying a file-encoding warning whatever else it found. sonar-project.properties
// answers that by excluding the binaries, stated by extension.
//
// That list is a census, and a census kept by hand fails short. This one did:
// it was written naming every binary in the tree, then a Word fixture arrived
// under a suffix it did not name, and the warning came back. So the corpus here
// is derived from the tree rather than restated — a tracked file that does not
// decode as UTF-8 IS the sensor's subject, by the sensor's own test — and a new
// binary enrols itself the moment it is committed, under any suffix at all.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"
)

// trackedBinaries returns the committed files the sensor would fail to read.
//
// Tracked files rather than a directory walk: the scan runs on a fresh
// checkout, so what is committed is exactly what it indexes, and a walk would
// also have to grow a skip-list for build output — the kind of prefilter that
// makes a census quietly read a smaller tree than it claims.
func trackedBinaries(t *testing.T) []string {
	t.Helper()
	out, err := exec.Command("git", "-C", repoRoot, "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this gate must run inside the git worktree)", err)
	}
	var binaries []string
	var read int
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" {
			continue
		}
		full := filepath.Join(repoRoot, rel)
		// A tracked path that is not a regular file is a submodule or a symlink,
		// which the sensor does not open. Asked before the read, and separately
		// from it, so that an unreadable REGULAR file fails below rather than
		// dropping out of the corpus — a census that skips what it cannot read
		// reports PASS for exactly the file it failed to examine.
		info, err := os.Lstat(full)
		if err != nil {
			t.Fatalf("%s is tracked but cannot be stat'd: %v", rel, err)
		}
		if !info.Mode().IsRegular() {
			continue
		}
		content, err := os.ReadFile(full)
		if err != nil {
			t.Fatalf("reading tracked file %s: %v", rel, err)
		}
		read++
		if !utf8.Valid(content) {
			binaries = append(binaries, rel)
		}
	}
	if read == 0 {
		t.Fatal("read no tracked file at all — a census over an empty tree reports PASS exactly like a clean one")
	}
	return binaries
}

// sonarExclusions is the sonar.exclusions value, split into its patterns.
// The property is written one pattern per line with a trailing backslash, so
// the continuations are joined before the commas are split.
func sonarExclusions(t *testing.T) []string {
	t.Helper()
	const property = "sonar.exclusions="
	raw, err := os.ReadFile(filepath.Join(repoRoot, "sonar-project.properties"))
	if err != nil {
		t.Fatalf("reading the scan configuration: %v", err)
	}
	joined := strings.ReplaceAll(string(raw), "\\\n", "")
	var value string
	for _, line := range strings.Split(joined, "\n") {
		if after, ok := strings.CutPrefix(line, property); ok {
			value = after
			break
		}
	}
	if value == "" {
		t.Fatalf("no %s in sonar-project.properties — the gate found nothing to hold", property)
	}
	var patterns []string
	for _, pattern := range strings.Split(value, ",") {
		if trimmed := strings.TrimSpace(pattern); trimmed != "" {
			patterns = append(patterns, trimmed)
		}
	}
	return patterns
}

// excludes answers whether one exclusion pattern covers a path, in the three
// shapes the property uses: a suffix rule, a subtree, and a single named file.
func excludes(pattern, path string) bool {
	if suffix, ok := strings.CutPrefix(pattern, "**/*"); ok {
		return strings.HasSuffix(path, suffix)
	}
	if subtree, ok := strings.CutSuffix(pattern, "/**"); ok {
		return strings.HasPrefix(path, subtree+"/")
	}
	return pattern == path
}

// TestEveryBinaryInTheTreeIsExcludedFromTheScan: a committed file the sensor
// cannot decode must be named by an exclusion, or the next analysis carries a
// file-encoding warning. The fix for a failure here is a suffix rule beside the
// others, not a path — the rule has to cover the next file of its kind too.
func TestEveryBinaryInTheTreeIsExcludedFromTheScan(t *testing.T) {
	t.Parallel()
	binaries := trackedBinaries(t)
	if len(binaries) == 0 {
		t.Fatal("found no binary file in the tree — this tree has always had some, so the census read something smaller than it claims")
	}
	patterns := sonarExclusions(t)

	var unexcluded []string
	for _, path := range binaries {
		covered := false
		for _, pattern := range patterns {
			if excludes(pattern, path) {
				covered = true
				break
			}
		}
		if !covered {
			unexcluded = append(unexcluded, path)
		}
	}
	if len(unexcluded) > 0 {
		t.Errorf("%d committed binary file(s) the scan would open as UTF-8 and warn on; add their suffix to sonar.exclusions:\n\t%s",
			len(unexcluded), strings.Join(unexcluded, "\n\t"))
	}
}

// TestTheExclusionShapesAreReadAsWritten pins the three pattern forms the
// property uses. Without it a parser that understood none of them would report
// every binary unexcluded, or — the direction that hides a defect — one that
// matched everything would report a tree with no exclusions at all as clean.
func TestTheExclusionShapesAreReadAsWritten(t *testing.T) {
	t.Parallel()
	cases := []struct {
		pattern, path string
		want          bool
	}{
		{"**/*.png", "docs/evidence/extension-tier/01-screen.png", true},
		{"**/*.wasm.module", "backend/internal/platform/licensecheck/module/licensecheck.wasm.module", true},
		{"**/*.png", "frontend/src/screens/testdata/proposal.docx", false},
		{"backend/migrations/**", "backend/migrations/core/0001.sql", true},
		{"backend/migrations/**", "backend/migrationsfoo/0001.sql", false},
		{".coderabbit.yaml", ".coderabbit.yaml", true},
		{".coderabbit.yaml", "nested/.coderabbit.yaml", false},
	}
	for _, c := range cases {
		if got := excludes(c.pattern, c.path); got != c.want {
			t.Errorf("excludes(%q, %q) = %v, want %v", c.pattern, c.path, got, c.want)
		}
	}
}
