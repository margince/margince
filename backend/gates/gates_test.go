// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package gates holds the fitness gates: the tests whose subject is the tree
// rather than a function. They live in one directory, apart from the code they
// judge, because they are not that code's unit tests — a gate here asks whether
// the module as a whole still holds an invariant, and answering that means
// walking packages none of them import.
//
// Every gate names its subject relative to the MODULE root — "internal/modules",
// "api/crm.yaml", "../docs/reference/gate-inventory.md" — because that is where
// the subjects are and where these files used to sit. Moving the files did not
// move their subjects, so TestMain restores that working directory rather than
// rewriting 117 files' worth of path literals. Many of those literals are
// import-path fragments a gate compares against and not file paths at all, so a
// mechanical rewrite would have corrupted the comparisons it did not change.
package gates

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"regexp/syntax"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// TestMain puts every gate back at the module root before any of them runs.
//
// It VERIFIES the landing rather than trusting the chdir, because the failure is
// silent in the direction that matters: a gate that walks "internal/modules"
// from the wrong directory finds nothing, and a census of nothing reports the
// clean tree it never read. Refusing to run at all is the only outcome that
// cannot be mistaken for a pass.
// repoRoot is the repository, from the directory TestMain leaves this package
// in. It is declared HERE, in the one file with no build constraint, rather than
// beside the first gate that needed it: a const behind `//go:build !integration`
// is invisible to `make lint`, which sets that tag — so every gate that used it
// compiled under `go test` and failed to compile under the linter, in a file
// that never mentioned the tag.
const repoRoot = ".."

// describesRatherThanRenders is a frontend file, named relative to
// frontend/src, that talks ABOUT what renders rather than rendering: a test, a
// story, a test kit, or the generated contract. Shared by the gates that sweep
// the frontend for a second rendering of something, so they read the same
// tree as production source.
func describesRatherThanRenders(rel string) bool {
	return strings.HasPrefix(rel, "api/") ||
		strings.Contains(rel, ".test.") ||
		strings.Contains(rel, ".stories.") ||
		strings.Contains(rel, ".testkit.") ||
		strings.HasSuffix(rel, ".d.ts")
}

func TestMain(m *testing.M) {
	if err := os.Chdir(".."); err != nil {
		fmt.Fprintf(os.Stderr, "gates: reaching the module root from the package directory: %v\n", err)
		os.Exit(1)
	}
	// The evidence is go.mod's MODULE LINE, not merely a go.mod. A bare
	// existence check would be satisfied by backend/tools/ and by every
	// directory under extensions/, each of which carries its own — so it would
	// answer yes for several wrong landings and only look like a proof.
	declaration, err := gatekit.ReadFile("go.mod")
	if err != nil {
		fmt.Fprintf(os.Stderr, "gates: reading go.mod to confirm the module root: %v\n", err)
		os.Exit(1)
	}
	if !bytes.HasPrefix(declaration, []byte("module "+modulePath+"\n")) {
		wd, wdErr := os.Getwd()
		if wdErr != nil {
			wd = "an unreadable directory"
		}
		fmt.Fprintf(os.Stderr, "gates: %s declares a different module than %s, so every gate here would "+
			"walk a tree that is not the one it judges and report the empty result as clean\n",
			wd, modulePath)
		os.Exit(1)
	}
	flag.Parse()
	if !rewritesTheTree() {
		gatekit.FixTree()
	}
	code := m.Run()
	if err := gatekit.CheckSharedBytes(); err != nil {
		fmt.Fprintf(os.Stderr, "gates: %v\n", err)
		code = 1
	}
	os.Exit(code)
}

// rewritesTheTree reports whether this run was given a flag of this package.
// Every one of them (-update-gate-inventory and its kind) rewrites a file that
// other gates read; the testing package's own flags rewrite nothing.
func rewritesTheTree() bool {
	rewrites := false
	flag.Visit(func(set *flag.Flag) {
		rewrites = rewrites || !strings.HasPrefix(set.Name, "test.")
	})
	return rewrites
}

// trackedFile is one entry from the index, with the one mode bit a gate that
// reads files cares about.
type trackedFile struct {
	path    string
	symlink bool
}

// trackedFiles reads the index. `-s` carries the mode, which is how a symlink is
// told from a file without stat-ing it; `-z` makes the output NUL-delimited, so
// no filename can be misread.
//
// It lives HERE, in the one file with no build constraint, for the reason
// repoRoot does: behind `//go:build !integration` it is invisible to `make lint`,
// which sets that tag, so a gate that used it compiled under `go test` and
// failed to compile under the linter.
func trackedFiles(t *testing.T) []trackedFile {
	t.Helper()
	files, err := trackedIndex()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this test must run inside the git worktree)", err)
	}
	return slices.Clone(files)
}

// trackedIndex is the index as one git call per test binary; the run does not
// change it.
var trackedIndex = sync.OnceValues(func() ([]trackedFile, error) {
	out, err := exec.Command("git", "-C", repoRoot, "ls-files", "-sz").Output()
	if err != nil {
		return nil, err
	}
	var files []trackedFile
	for row := range strings.SplitSeq(strings.TrimRight(string(out), "\x00"), "\x00") {
		if row == "" {
			continue
		}
		// <mode> <sha> <stage>\t<path>
		meta, path, ok := strings.Cut(row, "\t")
		if !ok {
			continue
		}
		files = append(files, trackedFile{path: path, symlink: strings.HasPrefix(meta, "120000")})
	}
	return files, nil
})

// requiredWords returns lower-case literals of which every match of re holds at
// least one, or nil when the pattern promises none.
func requiredWords(re *regexp.Regexp) []string {
	parsed, err := syntax.Parse(re.String(), syntax.Perl)
	if err != nil {
		return nil
	}
	return requiredWordsIn(parsed)
}

func requiredWordsIn(re *syntax.Regexp) []string {
	switch re.Op {
	case syntax.OpLiteral:
		return []string{strings.ToLower(string(re.Rune))}
	case syntax.OpCapture, syntax.OpPlus:
		return requiredWordsIn(re.Sub[0])
	case syntax.OpRepeat:
		if re.Min > 0 {
			return requiredWordsIn(re.Sub[0])
		}
	case syntax.OpConcat:
		// Any one part's words will do. The part whose shortest word is longest
		// rules out the most lines.
		var best []string
		for _, part := range re.Sub {
			if words := requiredWordsIn(part); shortestWord(words) > shortestWord(best) {
				best = words
			}
		}
		return best
	case syntax.OpAlternate:
		var all []string
		for _, branch := range re.Sub {
			words := requiredWordsIn(branch)
			if words == nil {
				return nil
			}
			all = append(all, words...)
		}
		return all
	}
	return nil
}

func shortestWord(words []string) int {
	if len(words) == 0 {
		return 0
	}
	shortest := len(words[0])
	for _, word := range words[1:] {
		shortest = min(shortest, len(word))
	}
	return shortest
}

// mayHoldWordsOf is the byte check in front of a pattern, given what
// requiredWords derived from it: nil words promise nothing, so the text is read.
func mayHoldWordsOf(text string, words []string) bool {
	return words == nil || mayHoldFolded(text, words...)
}

// foldsToASCIIUnlowered is the one rune (?i) matches to an ASCII letter that
// strings.ToLower leaves alone: ſ, which folds to s.
const foldsToASCIIUnlowered = 'ſ'

// mayHoldFolded is the cheap question in front of a case-insensitive scan. It
// is false only when text holds none of words in any case, so a scan it skips
// could not have matched.
func mayHoldFolded(text string, words ...string) bool {
	if strings.ContainsRune(text, foldsToASCIIUnlowered) {
		return true
	}
	folded := strings.ToLower(text)
	for _, word := range words {
		if strings.Contains(folded, word) {
			return true
		}
	}
	return false
}
