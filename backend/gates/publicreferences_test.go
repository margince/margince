// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

//go:build !integration

package gates

// This repository is public. Everything in it is readable by anyone, which
// makes a reference to a private repository, document or pull request two
// distinct failures at once: it leaks internal structure, and it sends a
// contributor somewhere they cannot go to satisfy a rule that then blocks
// their push.
//
// The rule is therefore absolute rather than a matter of taste: if a rule
// matters, it is written out here. A citation is by decision number, and the
// number is a label, not a pointer: the record it names is not in this tree.
//
// This derives the file list from git rather than walking the filesystem, so
// an untracked scratch file is out of scope and a newly tracked one is in it
// automatically. There is no exemption list: the one file that could not be
// cleaned was moved out of this repository instead, and an exemption is a hole
// somebody eventually widens.
//
// WHAT A GREEN RUN HERE DOES NOT MEAN. The rule covers commit messages and PR
// bodies as well as the tree, and no test can reach either — they are not files
// this or any checkout holds. That half stays judgement, and it is said here
// rather than left for a reader to assume: a gate that covers most of a rule
// and says nothing about the rest is read as covering all of it, which is how
// the half nobody checks becomes the half nobody thinks about.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"regexp"
	"regexp/syntax"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const privateProjectName = "constel" + "lation"

// registryPullTarget is the one literal path allowed to carry the private
// project's name: the image the release workflow pulls. It is matched
// case-sensitively and whole, so a URL, a link target or a capitalised form
// of the name still fails.
const registryPullTarget = "ghcr.io/gradionhq/margince-" + privateProjectName + "/release-management"

// forbidden is what a private reference looks like in this tree's history.
// Each pattern carries the plain-language reason it fails, because a gate that
// only says "no match allowed" leaves the reader guessing what to write.
var forbidden = []struct {
	name    string
	pattern *regexp.Regexp
	why     string
	// docsOnly limits a pattern to Markdown: the API contract and the release
	// workflow name these sources functionally, while a page only cites them.
	docsOnly bool
}{
	// The private project's name, bare or as a repository, in any case. It is
	// built from parts so this file does not spell it out. The one registry pull
	// target the release workflow needs is removed from a line before matching.
	{
		name:    "private repository name",
		pattern: regexp.MustCompile(`(?i)margince-(foundation|principles|business)|` + privateProjectName),
		why:     "name the thing, not the repository — a reader here cannot open it",
	},
	{
		name:    "private specification path",
		pattern: regexp.MustCompile(`(^|[^\w/.-])specs/[a-z]`),
		why:     "write the rule out here — a public contributor cannot open anything else",
	},
	{
		name:    "private pull-request reference",
		pattern: regexp.MustCompile(`foundation\s?((PR|pull request)\s*)?#\d+`),
		why:     "link an issue or PR in this repository, or state the fact without a link",
	},
	{
		name:     "private GitHub owner path",
		pattern:  regexp.MustCompile(`gradionhq/`),
		why:      "describe the repository by role (\"the private release repository\"); a reader here cannot open it",
		docsOnly: true,
	},
	{
		name:     "private specification document",
		pattern:  regexp.MustCompile(`business/\d+-|contract/interfaces\.md|(^|[^\w/.-])design/\d\d-[a-z-]+\.md|\b(data-model|formulas)(\.md)?\s*§`),
		why:      "write the rule out here — a public contributor cannot open the specification",
		docsOnly: true,
	},
}

// scanned are the file types whose prose a human or agent actually reads.
// Binary and generated artifacts are excluded: a match inside a lockfile or an
// image is noise, and generated files are regenerated rather than edited.
var scanned = map[string]bool{
	".go": true, ".md": true, ".yml": true, ".yaml": true,
	".json": true, ".ts": true, ".tsx": true, ".sh": true, ".sql": true,
	// .css because prose lives there too: a design-system file explains a
	// colour decision in a comment, and a comment is where a citation of an
	// unreachable document goes unnoticed. It was the last extension the
	// frontend writes in that this gate could not see.
	".css": true,
}

func TestPublicTreeCitesNothingPrivate(t *testing.T) {
	t.Parallel()
	out, err := exec.Command("git", "-C", "..", "ls-files", "-z").Output()
	if err != nil {
		t.Fatalf("listing tracked files: %v (this test must run inside the git worktree)", err)
	}

	census, named := 0, 0
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel == "" || !isScanned(rel) {
			continue
		}
		// This file names the patterns it bans, so scanning it would fail on
		// its own source. The exemption is the exact path, not the basename: a
		// second file called publicreferences_test.go anywhere else in the tree
		// is scanned like everything else.
		if rel == "backend/gates/publicreferences_test.go" {
			continue
		}
		assertFileCitesNothingPrivate(t, rel)
		census++
		if rel == "Makefile" || rel == "docker-bake.hcl" {
			named++
		}
	}
	if census < publicTreeFloor || named != 2 {
		t.Errorf("the gate read %d files (floor %d) and %d of the Makefile and docker-bake.hcl: "+
			"its census has shrunk and a private name could hide in what it no longer reads", census, publicTreeFloor, named)
	}
}

// publicTreeFloor is below the real count so ordinary deletions pass, and far above
// what a broken file filter would leave.
const publicTreeFloor = 15000

// isScanned also takes files with no extension and the .hcl build files.
// The Makefile and the bake configs carry prose in comments like any source.
func isScanned(rel string) bool {
	ext := filepath.Ext(rel)
	return ext == "" || ext == ".hcl" || scanned[ext]
}

// assertFileCitesNothingPrivate reports every offending line in one file, so a
// sweep is one fix-and-rerun cycle rather than one per line.
func assertFileCitesNothingPrivate(t *testing.T, rel string) {
	t.Helper()

	// The working copy, not the committed blob: a violation must fail before
	// it is committed, not after.
	body, err := gatekit.ReadFile(filepath.Join("..", rel))
	if err != nil {
		// A tracked file can be absent from the working tree mid-rebase or
		// after `git rm`. That is not this gate's business.
		if os.IsNotExist(err) {
			return
		}
		t.Fatalf("reading %s: %v", rel, err)
	}

	for i, line := range strings.Split(string(body), "\n") {
		shown := line
		line = strings.ReplaceAll(line, registryPullTarget, "")
		for p, f := range forbidden {
			if f.docsOnly && filepath.Ext(rel) != ".md" {
				continue
			}
			if mayMatchForbidden(p, line) && f.pattern.MatchString(line) {
				t.Errorf("%s:%d carries a %s — %s\n\t%s",
					rel, i+1, f.name, f.why, strings.TrimSpace(shown))
			}
		}
	}
}

// forbiddenWords holds, index for index with forbidden, the words every match of
// that pattern contains. Derived from the pattern, so an edit to one cannot
// leave a stale list beside it.
var forbiddenWords = func() [][]string {
	words := make([][]string, len(forbidden))
	for i, f := range forbidden {
		words[i] = requiredWords(f.pattern)
	}
	return words
}()

// mayMatchForbidden is false only when line holds none of the words pattern i
// requires. A pattern that requires none is always asked.
func mayMatchForbidden(i int, line string) bool {
	return forbiddenWords[i] == nil || mayHoldFolded(line, forbiddenWords[i]...)
}

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

func TestRequiredWordsAreWordsEveryMatchHolds(t *testing.T) {
	t.Parallel()
	for pattern, want := range map[string][]string{
		`specs/`:               {"specs/"},
		`(?i)Margince-`:        {"margince-"},
		`foo|bar`:              {"foo", "bar"},
		`(^|[^\w])specs/[a-z]`: {"specs/"},
		`a\d+longer`:           {"longer"},
		`(ab|cd)e+`:            {"ab", "cd"},
		`x{2,}`:                {"x"},
	} {
		if got := requiredWords(regexp.MustCompile(pattern)); !slices.Equal(got, want) {
			t.Errorf("requiredWords(%q) = %q, want %q", pattern, got, want)
		}
	}
	// A pattern a line can match without any fixed word promises none, so the
	// regex must always run.
	for _, pattern := range []string{`a?b?`, `[ab]+`, `a|\d`, `.*`, `x{0,3}`} {
		if got := requiredWords(regexp.MustCompile(pattern)); got != nil {
			t.Errorf("requiredWords(%q) = %q, but a match need hold none of them", pattern, got)
		}
	}
}

func TestEveryForbiddenPatternIsAskedOfTheLinesItMatches(t *testing.T) {
	t.Parallel()
	samples := map[string]string{
		"private repository name":        "see the " + strings.ToUpper(privateProjectName) + " repository",
		"private specification path":     "read specs/billing first",
		"private pull-request reference": "as foundation PR #12 decided",
		"private GitHub owner path":      "cloned from gradionhq/tools",
		"private specification document": "per formulas.md § 3",
	}
	for i, f := range forbidden {
		if forbiddenWords[i] == nil {
			t.Errorf("%s: its pattern promises no word, so every line pays for its regex", f.name)
		}
		line, ok := samples[f.name]
		if !ok {
			t.Errorf("%s has no sample line here; add one that the pattern matches", f.name)
			continue
		}
		if !f.pattern.MatchString(line) {
			t.Errorf("%s: the sample %q does not match the pattern, so it proves nothing", f.name, line)
		}
		if !mayMatchForbidden(i, line) {
			t.Errorf("%s: the words %q skip %q, a line the pattern matches", f.name, forbiddenWords[i], line)
		}
	}
}

// A document git ignores is the same dead pointer as a private repository, and
// it arrives by a route the patterns above cannot see: no name gives it away.
// Four accumulated in tracked prose with every gate green.
//
// THE DISTINCTION IS WHAT IS CITED, not where it sits. A gitignored path a
// COMMAND WRITES is a fact worth documenting — "artifacts land in the
// gitignored aitask directory" tells a reader exactly what to expect, and they
// get the directory by running the command. A gitignored DOCUMENT is different:
// there is nothing to run, the reader cannot produce it, and the sentence
// citing it is asking them to read something that does not exist in their
// clone. So this asks only about `.md` citations, which is why it can be
// absolute where a rule about every ignored path would have to be a judgement.
//
// GIT ANSWERS, rather than a list of ignored roots kept here. A list is a
// second copy of .gitignore, and the copy that stopped matching would let the
// next scratch root through silently.

// citedDocument matches a Markdown path as prose writes one — in backticks, in
// a link, or bare. The extension is what bounds it: without one this would have
// to guess where a sentence stops being a path.
var citedDocument = regexp.MustCompile(`[\w.][\w./-]*\.md\b`)

func TestPublicTreeCitesNoDocumentGitIgnores(t *testing.T) {
	t.Parallel()
	tracked := trackedPaths(t)

	type citation struct {
		rel  string
		line int
		text string
	}
	cited := map[string][]citation{}
	for rel := range tracked {
		// MARKDOWN ONLY, and the boundary is not laziness. The obligation is
		// that a reader can follow what this repository TELLS them, and what it
		// tells them is prose. A path inside a Go string literal is usually a
		// fixture building a temp tree — the gen-composition tests write an
		// AGENTS.md under an extensions/ path that git ignores, and nothing is
		// wrong with that — and a fixture cannot be told from a citation by
		// looking at it. Reporting those would either add a waiver list or teach
		// contacts to spell paths so this gate cannot see them.
		if filepath.Ext(rel) != ".md" {
			continue
		}
		body, err := gatekit.ReadFile(filepath.Join("..", rel))
		if err != nil {
			// Absent mid-rebase or after `git rm`; not this gate's business.
			if os.IsNotExist(err) {
				continue
			}
			t.Fatalf("reading %s: %v", rel, err)
		}
		for i, line := range strings.Split(string(body), "\n") {
			for _, cand := range citedDocument.FindAllString(line, -1) {
				for _, resolved := range unresolvedTargets(rel, cand, tracked) {
					cited[resolved] = append(cited[resolved],
						citation{rel: rel, line: i + 1, text: line})
				}
			}
		}
	}
	if len(cited) == 0 {
		t.Fatal("no Markdown path is cited anywhere in tracked prose — this gate has stopped " +
			"seeing its subject rather than the tree having stopped citing documents")
	}

	// Reported once per SITE, not once per reading: a citation that is ignored
	// under both the relative and the root reading is one mistake, and printing
	// it twice makes a four-line sweep look like an eight-line one.
	said := map[string]bool{}
	for _, ignored := range gitIgnores(t, cited) {
		for _, c := range cited[ignored] {
			where := fmt.Sprintf("%s:%d", c.rel, c.line)
			if said[where] {
				continue
			}
			said[where] = true
			t.Errorf("%s cites a document git ignores — it does not exist in a fresh clone, so "+
				"a reader cannot follow it. State the fact here, or cite something this repository "+
				"ships.\n\t%s", where, strings.TrimSpace(c.text))
		}
	}
}

// unresolvedTargets is where a cited path could point, minus everywhere it
// resolves to something this repository ships.
//
// Prose writes a Markdown link BOTH ways — relative to the file it sits in, and
// from the repository root — and neither spelling is wrong. So both are tried,
// and a citation that lands on a tracked file under EITHER reading is settled
// without asking git, which is the ordinary case and by far the common one.
//
// A reading that climbs out of the repository is dropped rather than reported:
// a nested module citing ../../AGENTS.md is naming the root file, and the
// join above already produced that reading. What is left over points outside
// the tree, which git cannot answer about and this gate does not ask.
func unresolvedTargets(rel, cand string, tracked map[string]bool) []string {
	readings := []string{path.Clean(cand), path.Clean(path.Join(path.Dir(rel), cand))}
	var out []string
	for _, r := range readings {
		if tracked[r] {
			return nil
		}
		if r == ".." || strings.HasPrefix(r, "../") || path.IsAbs(r) || slices.Contains(out, r) {
			continue
		}
		out = append(out, r)
	}
	return out
}

// trackedPaths is the index as a set. It reads the shared trackedFiles helper
// rather than shelling out again: one spelling of "what does git know about".
func trackedPaths(t *testing.T) map[string]bool {
	t.Helper()
	tracked := map[string]bool{}
	for _, f := range trackedFiles(t) {
		tracked[f.path] = true
	}
	return tracked
}

// gitIgnores asks git which of these paths it excludes, in one call.
//
// check-ignore exits 1 when it matches nothing, which is the ordinary clean
// answer and not a failure — so the exit status is read through the output
// rather than treated as an error.
func gitIgnores[T any](t *testing.T, paths map[string][]T) []string {
	t.Helper()
	candidates := make([]string, 0, len(paths))
	for path := range paths {
		candidates = append(candidates, path)
	}
	sort.Strings(candidates)

	cmd := exec.Command("git", "-C", "..", "check-ignore", "--stdin")
	cmd.Stdin = strings.NewReader(strings.Join(candidates, "\n") + "\n")
	out, err := cmd.Output()
	if err != nil {
		var exit *exec.ExitError
		// 1 is "nothing matched". Anything else is git failing to answer, and a
		// gate that read that as a clean tree would report PASS over an
		// unanswered question.
		if !errors.As(err, &exit) || exit.ExitCode() != 1 {
			t.Fatalf("asking git which cited documents it ignores: %v", err)
		}
	}
	var ignored []string
	for _, path := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if path != "" {
			ignored = append(ignored, path)
		}
	}
	sort.Strings(ignored)
	return ignored
}
