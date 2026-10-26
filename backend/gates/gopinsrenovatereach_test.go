// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H3

//go:build !integration

package gates

// Every Go pin this tree holds is one Renovate can move, and it moves them
// together.
//
// TestEveryGoVersionPinMatchesTheProductModule compares fifteen pins against
// each other. Renovate's asdf manager owns exactly one of them, and gomod's
// `go` directive was disabled outright — so a bump moved `.tool-versions`,
// left the other fourteen behind, and took `deterministic-gates` and through it
// `ci` red on every open pull request the moment it was rebased. Twice.
//
// The version gate is the wrong place to catch that: by the time it fails the
// half-moved tree is already on main. This one asks the other question, of the
// configuration rather than of the versions — can the bot reach every pin, and
// does it raise them in one branch.
//
// Asked of the live file rather than of a copy of its rules: the manager's own
// pattern is run against the document it claims to own, so a pattern that
// stopped matching fails here rather than going quiet and shipping a stale pin.

import (
	"encoding/json"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// renovateConfig is the shape this gate judges and nothing more.
type renovateConfig struct {
	PackageRules []struct {
		Enabled           *bool    `json:"enabled"`
		GroupName         string   `json:"groupName"`
		MatchManagers     []string `json:"matchManagers"`
		MatchDepTypes     []string `json:"matchDepTypes"`
		MatchPackageNames []string `json:"matchPackageNames"`
	} `json:"packageRules"`
	CustomManagers []struct {
		ManagerFilePatterns []string `json:"managerFilePatterns"`
		MatchStrings        []string `json:"matchStrings"`
	} `json:"customManagers"`
}

// extensionHowTo is the one Go pin no built-in manager owns: a go.mod shown
// verbatim in a fenced block, which whoever writes the next extension copies.
const extensionHowTo = "docs/how-to/add-an-extension.md"

func readRenovateConfig(t *testing.T) renovateConfig {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(repoRoot, "renovate.json"))
	if err != nil {
		t.Fatalf("reading renovate.json: %v", err)
	}
	var cfg renovateConfig
	if err := json.Unmarshal(raw, &cfg); err != nil {
		t.Fatalf("parsing renovate.json: %v", err)
	}
	if len(cfg.PackageRules) == 0 {
		t.Fatal("renovate.json declares no packageRules, so this gate is reading a file it does not understand")
	}
	return cfg
}

// The gomod manager must be allowed to raise the `go` directive. Disabling it
// is what left fourteen pins with no bumper at all.
func TestRenovateMayMoveTheGoDirective(t *testing.T) {
	t.Parallel()
	for _, rule := range readRenovateConfig(t).PackageRules {
		if rule.Enabled == nil || *rule.Enabled {
			continue
		}
		if slices.Contains(rule.MatchManagers, "gomod") && slices.Contains(rule.MatchDepTypes, "golang") {
			t.Error("a packageRule disables gomod's `golang` depType, so nothing raises the `go` directive in " +
				"any module. The asdf manager still bumps .tool-versions alone, which is the half-moved tree " +
				"TestEveryGoVersionPinMatchesTheProductModule fails on — after it has merged.")
		}
	}
}

// One branch, or the tree is half-moved for as long as the second pull request
// takes to open.
func TestRenovateRaisesEveryGoPinInOneBranch(t *testing.T) {
	t.Parallel()
	for _, rule := range readRenovateConfig(t).PackageRules {
		if rule.GroupName == "" || !slices.Contains(rule.MatchPackageNames, "golang") {
			continue
		}
		for _, manager := range []string{"gomod", "asdf", "custom.regex"} {
			if !slices.Contains(rule.MatchManagers, manager) {
				t.Errorf("the %q group leaves out the %s manager, so the pins it owns arrive in a "+
					"branch of their own and every pin outside it is behind until that one merges too.",
					rule.GroupName, manager)
			}
		}
		return
	}
	t.Error("no packageRule groups the go pins, so each manager raises its own branch and every one of " +
		"them fails the version gate until the last merges.")
}

// The how-to's own pin, asked of the document: a custom manager whose pattern
// has stopped matching reports nothing and ships a stale pin, which is the
// direction this must not fail.
func TestTheExtensionHowToPinIsOneRenovateCanFind(t *testing.T) {
	t.Parallel()
	doc, err := os.ReadFile(filepath.Join(repoRoot, extensionHowTo))
	if err != nil {
		t.Fatalf("reading %s: %v", extensionHowTo, err)
	}
	for _, manager := range readRenovateConfig(t).CustomManagers {
		if !anyMatches(t, manager.ManagerFilePatterns, extensionHowTo) {
			continue
		}
		for _, matchString := range manager.MatchStrings {
			if goPinNamed(t, matchString).MatchString(string(doc)) {
				return
			}
		}
		t.Errorf("a custom manager claims %s and none of its matchStrings finds a `go` pin in it. "+
			"Renovate reports nothing for a manager that matches nothing, so the pin stays behind and "+
			"the version gate fails on a tree that already merged.", extensionHowTo)
		return
	}
	t.Errorf("no custom manager claims %s, and no built-in manager owns a fenced go.mod — so the pin "+
		"whoever writes the next extension copies has no bumper.", extensionHowTo)
}

// goPinNamed compiles one of Renovate's matchStrings for Go's regexp engine.
// Renovate runs JavaScript, where a named group is `(?<name>…)`; RE2 spells the
// same thing `(?P<name>…)` and rejects the other. Only the spelling differs, so
// translating it reads the real pattern rather than a paraphrase of it.
func goPinNamed(t *testing.T, matchString string) *regexp.Regexp {
	t.Helper()
	pattern, err := regexp.Compile(strings.ReplaceAll(matchString, "(?<", "(?P<"))
	if err != nil {
		t.Fatalf("compiling the custom manager's matchString %q: %v", matchString, err)
	}
	return pattern
}

func anyMatches(t *testing.T, patterns []string, file string) bool {
	t.Helper()
	for _, pattern := range patterns {
		if reachesLikeRenovate(t, pattern, file) {
			return true
		}
	}
	return false
}

// reachesLikeRenovate answers whether Renovate would read `pattern` as naming
// `file`. That is not Go's regexp question.
//
// Renovate takes a pattern as a regex only between slashes, and as a minimatch
// glob otherwise. Compiling every pattern as a regex is what let
// `^docs/how-to/add-an-extension\.md$` satisfy this gate while naming no
// file: as a glob, `^` and `\.` are literals no path carries.
//
// path.Match is not minimatch. It has no `**`, so a pattern crossing
// directories answers false here. The gate then asks for a regex, which is
// the stricter way to be wrong.
func reachesLikeRenovate(t *testing.T, pattern, file string) bool {
	t.Helper()
	if len(pattern) > 1 && strings.HasPrefix(pattern, "/") && strings.HasSuffix(pattern, "/") {
		re, err := regexp.Compile(pattern[1 : len(pattern)-1])
		if err != nil {
			t.Fatalf("compiling the custom manager's file pattern %q as a regex: %v", pattern, err)
		}
		return re.MatchString(file)
	}
	matched, err := path.Match(pattern, file)
	if err != nil {
		t.Fatalf("reading the custom manager's file pattern %q as a glob: %v", pattern, err)
	}
	return matched
}

// The Makefile's digest pins, asked of the Makefile rather than of a list.
//
// Six SBOM tools are pinned there, and Renovate has no manager for a
// Makefile. So each is a hand edit nothing prompts until a custom manager
// claims the file: syft sat on v1.50.0 after v1.51.1 fixed a pnpm-lock read.
//
// The corpus is every `*_IMAGE` line the Makefile carries. A seventh pin
// fails this by being written rather than by being remembered.
// assignmentIn names the Makefile variable one regex hit covers. Credit then
// lands on that pin, not on one whose name merely ends the same way.
//
// It reads the hit's `name` group when the manager captures one. Otherwise it
// falls back to the leading assignment, which keeps a manager written without
// that group working.
func assignmentIn(hit []string) string {
	leading := regexp.MustCompile(`([A-Z_]+_IMAGE)[^\S\r\n]*\?=`)
	for _, group := range hit[1:] {
		if leading.MatchString(group + "?=") {
			return group
		}
	}
	if found := leading.FindStringSubmatch(hit[0]); found != nil {
		return found[1]
	}
	return ""
}

func TestEveryMakefileImagePinIsOneRenovateCanOffer(t *testing.T) {
	t.Parallel()
	const makefile = "Makefile"
	text, err := os.ReadFile(filepath.Join(repoRoot, makefile))
	if err != nil {
		t.Fatalf("reading %s: %v", makefile, err)
	}
	pinned := regexp.MustCompile(`(?m)^([A-Z_]+_IMAGE)\s*\?=`).FindAllStringSubmatch(string(text), -1)
	if len(pinned) == 0 {
		t.Fatal("no *_IMAGE pin in the Makefile, so this gate reads a corpus that " +
			"has moved and would pass over every pin there is")
	}
	claimed := map[string]bool{}
	for _, manager := range readRenovateConfig(t).CustomManagers {
		if !anyMatches(t, manager.ManagerFilePatterns, makefile) {
			continue
		}
		for _, matchString := range manager.MatchStrings {
			for _, hit := range goPinNamed(t, matchString).FindAllStringSubmatch(string(text), -1) {
				// The assignment the hit covers, read back from the hit itself.
				// Asking whether a hit contains a pin name credits the wrong
				// one. A hit on SBOM_PYTHON_IMAGE contains PYTHON_IMAGE, so a
				// pin by that name would read as covered by it.
				claimed[assignmentIn(hit)] = true
			}
		}
	}
	for _, pin := range pinned {
		if !claimed[pin[1]] {
			t.Errorf("%s is pinned by digest in the Makefile and no custom manager offers it an "+
				"update, so it moves only when somebody edits it by hand", pin[1])
		}
	}
}

// TestAnImagePinReadsOnlyItsOwnLine holds the Makefile manager's whitespace to
// one line. Renovate applies a matchString to the whole file, so a run of `\s`
// crosses the newline.
//
// A pin whose tag comment were removed would then take the next line's comment
// as its version. Renovate would offer an update against a tag naming
// something else.
func TestAnImagePinReadsOnlyItsOwnLine(t *testing.T) {
	t.Parallel()
	const stripped = "SBOM_PYTHON_IMAGE ?= python@sha256:abc123\n# Per-format notes\n"
	for _, manager := range readRenovateConfig(t).CustomManagers {
		if !anyMatches(t, manager.ManagerFilePatterns, "Makefile") {
			continue
		}
		for _, matchString := range manager.MatchStrings {
			if hit := goPinNamed(t, matchString).FindString(stripped); hit != "" {
				t.Errorf("the Makefile manager matched %q across a newline, so a pin "+
					"missing its tag comment takes the following line as its version", hit)
			}
		}
	}
}
