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

func anyMatches(t *testing.T, patterns []string, path string) bool {
	t.Helper()
	for _, pattern := range patterns {
		matched, err := regexp.MatchString(pattern, path)
		if err != nil {
			t.Fatalf("compiling the custom manager's file pattern %q: %v", pattern, err)
		}
		if matched {
			return true
		}
	}
	return false
}
