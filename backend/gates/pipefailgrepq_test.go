// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A shell gate does not decide its verdict through a pipe that can break.
//
// `printf '%s' "$body" | grep -q PATTERN` under `set -o pipefail` is a RACE.
// `grep -q` exits the moment it matches, which closes the pipe while printf is
// still writing; printf then fails with EPIPE, pipefail promotes that to the
// pipeline's status, and the caller reads a match as no-match. Whether it
// happens depends on how much the producer had left to write and how fast the
// consumer exited — so it passes on a laptop and fails on a runner, on the same
// commit.
//
// It cost a merge. `scripts/test-scheduled-report.sh` reported
//
//	FAIL: MAIN_SONAR_RESULT has a reporter arm, but main-health's report job
//	does not run for a failing 'sonar' — the arm is unreachable
//
// on a job whose `if:` names that very lane, one line after
// `printf: write error: Broken pipe`, while the same script was green locally.
// Ten more sites across the script gates had the same shape.
//
// A here-string has no second process and cannot break: `grep -q PATTERN
// <<<"$body"`. Every one of these scripts is bash, so the spelling is available
// wherever the shape appears.
//
// SCOPE, and why it stops here: only a producer that is the SHELL ITSELF —
// `printf` or `echo` of a variable — is judged. An external command on the left
// (`git log … | grep -q`) can also take SIGPIPE, but it is a different fix with
// a much larger surface, and the shell-side sites are the ones that carry a
// whole variable's worth of output into a consumer that stops early.

import (
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const shellGateDir = "../scripts"

// `printf '%s' "$var" | grep -q…` or `echo "…$var…" | grep -q…`, which is the
// shape that broke. A pipe into a `grep` without -q consumes all of its input
// and does not race.
//
// Both quotings of the format string, and a variable ANYWHERE in the operand:
// `printf "%s"` is the same producer as `printf '%s'`, and `echo "at $sha"`
// writes just as much into a consumer that stops early. A prohibition that
// recognised one spelling would be avoidable by using the other, which is not a
// property a prohibition may have.
var shellVariableIntoQuietGrep = regexp.MustCompile(
	`(?:printf\s+["']%s(?:\\n)?["']|echo)\s+"[^"]*\$\{?[A-Za-z_][A-Za-z0-9_]*\}?[^"]*"\s*\|\s*grep\s+-[A-Za-z]*q`)

// The setting that turns a broken pipe into the pipeline's verdict — recognised
// by the word rather than by the flag letters around it. `set -Eeuo pipefail`,
// `set -e -o pipefail` and `set -o pipefail` are the same setting, and a
// prohibition that recognised only one spelling would let a script escape it by
// being written the other way. Nothing but this setting says `pipefail`.
var pipefailSetting = regexp.MustCompile(`(?m)^\s*set\s+.*\bpipefail\b`)

func TestNoShellGateReadsAVerdictThroughAPipeThatCanBreak(t *testing.T) {
	t.Parallel()

	// The whole tree, not its top level. `scripts/deploy/` already holds two
	// entrypoints, and a glob of `*.sh` judged neither — an under-scoped census
	// reports a pass over the part it read, and the empty-tree guard below
	// cannot see the difference between "nothing there" and "nothing looked at".
	var scripts []string
	err := filepath.WalkDir(shellGateDir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() && strings.HasSuffix(path, ".sh") {
			scripts = append(scripts, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", shellGateDir, err)
	}
	if len(scripts) == 0 {
		t.Fatalf("%s holds no *.sh — this prohibition read an empty tree", shellGateDir)
	}
	// And it reaches BELOW the top level, which is the scoping the glob got
	// wrong: a tree with subdirectories that this walk flattens to one level
	// would pass every assertion below while judging none of them.
	nested := 0
	for _, path := range scripts {
		if strings.Count(filepath.ToSlash(path), "/") > strings.Count(filepath.ToSlash(shellGateDir), "/")+1 {
			nested++
		}
	}
	if nested == 0 {
		t.Errorf("the walk found no *.sh below the top level of %s, and scripts/deploy/ holds two — the census is reading one directory where it means the tree", shellGateDir)
	}
	judged := 0
	for _, path := range scripts {
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		text := string(body)
		if !pipefailSetting.MatchString(text) {
			continue
		}
		judged++
		for number, line := range strings.Split(text, "\n") {
			if !shellVariableIntoQuietGrep.MatchString(line) {
				continue
			}
			t.Errorf("%s:%d decides a verdict through a pipe that can break under pipefail:\n\t%s\n"+
				"`grep -q` exits on its first match and the producer then fails with EPIPE, so a match reads as no-match — on some machines and not others. Use a here-string: grep -q… <<<\"$var\".",
				path, number+1, strings.TrimSpace(line))
		}
	}
	if judged == 0 {
		t.Fatalf("no script under %s sets pipefail, which is not a shape this tree has ever had — the setting is spelled some way this gate does not recognise", shellGateDir)
	}
}

func TestEverySpellingOfPipefailIsRecognised(t *testing.T) {
	t.Parallel()

	// One setting, several spellings, and the gate above judges only the scripts
	// where it is on. A matcher that missed a spelling would leave those scripts
	// unjudged and say nothing — under-recognition, which is the same defect
	// class the prohibition itself is about.
	for setting, want := range map[string]bool{
		"set -euo pipefail":   true,
		"set -Eeuo pipefail":  true,
		"set -e -o pipefail":  true,
		"set -o pipefail":     true,
		"\tset -eo pipefail":  true,
		"set -eu":             false,
		"# set -euo pipefail": false,
		"echo 'set pipefail'": false,
	} {
		if got := pipefailSetting.MatchString(setting); got != want {
			t.Errorf("pipefailSetting.MatchString(%q) = %v, want %v", setting, got, want)
		}
	}
}

func TestEverySpellingOfTheBreakablePipeIsRecognised(t *testing.T) {
	t.Parallel()

	// The shape, not one way of writing it. Each false case is a pipe that does
	// NOT race: a grep without -q reads its input to the end.
	for line, want := range map[string]bool{
		`printf '%s' "$body" | grep -q X`:    true,
		`printf "%s" "$body" | grep -q X`:    true,
		`printf '%s\n' "$body" | grep -qF X`: true,
		`echo "$body" | grep -Eq X`:          true,
		`echo "at $sha" | grep -q X`:         true,
		`printf '%s' "$body" | grep X`:       false,
		`git log -1 | grep -q X`:             false,
		`printf '%s' "$body" > "$out"`:       false,
	} {
		if got := shellVariableIntoQuietGrep.MatchString(line); got != want {
			t.Errorf("shellVariableIntoQuietGrep.MatchString(%q) = %v, want %v", line, got, want)
		}
	}
}
