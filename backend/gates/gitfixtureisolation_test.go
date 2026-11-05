// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Every test that builds a git repository keeps the caller's git config out of it.
//
// A global commit.gpgsign otherwise fails the fixture's commits on a developer's
// machine while CI stays green. Shell tests source scripts/lib-git-fixture.sh;
// tests in other languages set GIT_CONFIG_GLOBAL in the environment they hand git.
//
// Blind spots: a subcommand held in a variable, and a git call split over lines.
// A test file outside the naming conventions below is not read.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const gitFixtureLib = "scripts/lib-git-fixture.sh"

// shellBuildsGitRepo matches a shell git call that creates a repository or a
// commit, with any -C <dir> or -c <key=value> options before the subcommand.
var shellBuildsGitRepo = regexp.MustCompile(`(^|[^\w-])git(\s+-[Cc]\s+\S+)*\s+(init|commit)\b`)

// argBuildsGitRepo matches the argument another language hands git: a string
// that opens with the subcommand, as in exec.Command("git", "init") or
// git("-c core.hooksPath=/dev/null commit -m x"). A string that merely names
// `git commit` in prose or in a pattern does not open with it.
var argBuildsGitRepo = regexp.MustCompile(`["'` + "`" + `](-c\s+\S+\s+)*(init|commit)(\s|["'` + "`" + `])`)

// spawnsGit matches another language starting git as a process. A file that
// never does is not building a repository, whatever its strings say.
var spawnsGit = regexp.MustCompile(`\b(Command|CommandContext|execFileSync|execFile|spawnSync|spawn|execSync|execa)\(\s*(\w+,\s*)?["'` + "`" + `]git\b`)

var sourcesGitFixtureLib = regexp.MustCompile(`(^|\s)(source|\.)\s+\S*lib-git-fixture\.sh\b`)

func isShellTest(base string) bool {
	return strings.HasSuffix(base, ".sh") && (strings.HasPrefix(base, "test-") || strings.HasSuffix(base, ".test.sh"))
}

func isTestFile(base string) bool {
	if isShellTest(base) || strings.HasSuffix(base, "_test.go") {
		return true
	}
	for _, suffix := range []string{".test.ts", ".test.tsx", ".test.js", ".test.mjs", ".spec.ts"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	return false
}

// isCommentLine covers the comment markers of the three languages read here.
func isCommentLine(code string) bool {
	return strings.HasPrefix(code, "#") || strings.HasPrefix(code, "//") || strings.HasPrefix(code, "*") || strings.HasPrefix(code, "/*")
}

func TestEveryTestThatBuildsAGitRepoIgnoresTheCallersGitConfig(t *testing.T) {
	t.Parallel()
	shellBuilders, otherBuilders := 0, 0
	for _, file := range trackedFiles(t) {
		base := filepath.Base(file.path)
		if file.symlink || !isTestFile(base) || file.path == "backend/gates/gitfixtureisolation_test.go" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(repoRoot, file.path)) // #nosec G304 -- a tracked path from the index
		if err != nil {
			t.Fatalf("reading %s: %v", file.path, err)
		}
		buildsGitRepo := argBuildsGitRepo
		if isShellTest(base) {
			buildsGitRepo = shellBuildsGitRepo
		}
		builds, sources := false, false
		for line := range strings.SplitSeq(string(raw), "\n") {
			code := strings.TrimSpace(line)
			if isCommentLine(code) {
				continue
			}
			builds = builds || buildsGitRepo.MatchString(code)
			sources = sources || sourcesGitFixtureLib.MatchString(code)
		}
		if !isShellTest(base) && !spawnsGit.Match(raw) {
			builds = false
		}
		switch {
		case !builds:
		case isShellTest(base):
			shellBuilders++
			if !sources {
				t.Errorf("%s builds a git repository but does not source %s: the fixture then "+
					"inherits the caller's git config, and a global commit.gpgsign or hooks path "+
					"fails the test on a developer's machine. Add `source \"$(pwd)/%s\"` before the first git call.",
					file.path, gitFixtureLib, gitFixtureLib)
			}
		default:
			otherBuilders++
			if !strings.Contains(string(raw), "GIT_CONFIG_GLOBAL") {
				t.Errorf("%s builds a git repository but never sets GIT_CONFIG_GLOBAL: the fixture then "+
					"inherits the caller's git config, and a global commit.gpgsign fails the test on a "+
					"developer's machine. Hand git an environment with GIT_CONFIG_GLOBAL=/dev/null and "+
					"GIT_CONFIG_SYSTEM=/dev/null, as %s does for the shell tests.", file.path, gitFixtureLib)
			}
		}
	}
	// One floor per half of the scan, set below the real counts. They catch a scan
	// that went blind, not a tree that changed.
	if shellBuilders < 3 || otherBuilders < 1 {
		t.Fatalf("found %d shell and %d other test(s) that build a git repository; the file-name "+
			"filter or the git pattern no longer sees its subject", shellBuilders, otherBuilders)
	}
}
