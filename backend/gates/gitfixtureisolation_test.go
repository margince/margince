// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind shape H1

package gates

// Every test that builds a git repository keeps the caller's git config out of it.
//
// A global commit.gpgsign otherwise fails the fixture's commits on a developer's
// machine while CI stays green. Shell tests source scripts/lib-git-fixture.sh
// before any git call, since an exported GIT_DIR steers every one. Other tests
// hand git GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM.
//
// Blind spots: a subcommand held in a variable, and a git call split over lines.
// A test file outside the naming conventions below is not read.

import (
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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

// shellCallsGit matches git started as a shell command, not the word in prose.
var shellCallsGit = regexp.MustCompile(`(^|[;&|(` + "`" + `!{]|\b(if|then|else|elif|do|while|until)\s)\s*git\s`)

var sourcesGitFixtureLib = regexp.MustCompile(`(^|\s)(source|\.)\s+.*lib-git-fixture\.sh\b`)

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
		raw, err := gatekit.ReadFile(filepath.Join(repoRoot, file.path)) // #nosec G304 -- a tracked path from the index
		if err != nil {
			t.Fatalf("reading %s: %v", file.path, err)
		}
		shell := isShellTest(base)
		if !shell && !spawnsGit.Match(raw) {
			continue
		}
		buildsGitRepo := argBuildsGitRepo
		if shell {
			buildsGitRepo = shellBuildsGitRepo
		}
		builds, sources, calledGitFirst, setsGlobal, setsSystem := false, false, false, false, false
		for line := range strings.SplitSeq(string(raw), "\n") {
			code := strings.TrimSpace(line)
			if isCommentLine(code) {
				continue
			}
			calledGitFirst = calledGitFirst || (!sources && shellCallsGit.MatchString(code))
			builds = builds || buildsGitRepo.MatchString(code)
			sources = sources || sourcesGitFixtureLib.MatchString(code)
			setsGlobal = setsGlobal || strings.Contains(code, "GIT_CONFIG_GLOBAL")
			setsSystem = setsSystem || strings.Contains(code, "GIT_CONFIG_SYSTEM")
		}
		switch {
		case !builds:
		case shell:
			shellBuilders++
			if !sources || calledGitFirst {
				t.Errorf("%s builds a git repository but does not source %s before its first git call: "+
					"the fixture then inherits the caller's git config and GIT_DIR, and a global "+
					"commit.gpgsign or hooks path fails the test on a developer's machine. Source it "+
					"from the script's own directory, not from a path git answered.",
					file.path, gitFixtureLib)
			}
		default:
			otherBuilders++
			if !setsGlobal || !setsSystem {
				t.Errorf("%s builds a git repository but does not set both GIT_CONFIG_GLOBAL and GIT_CONFIG_SYSTEM: the fixture then "+
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
