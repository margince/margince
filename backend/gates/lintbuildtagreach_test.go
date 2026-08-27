// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every build tag that gates a Go file in this repository is carried by both
// golangci configs, and both run govet.
//
// A tagged file is invisible to a pass that does not carry its tag. `make lint`
// is the only pass in the merge gate that carries any: `make build`, `make vet`
// and `make test` all run untagged, and the integration lane runs `-tags
// integration` alone. So for every tag but `integration`, the golangci
// build-tags list is the whole of that file's coverage — and a tag missing from
// it means those files are compiled by nothing the gate runs.
//
// That is not a hypothetical failure mode. It is how a file reached this tree
// referring to a package it does not import: no pass carried its tag, so
// nothing ever type-checked it, and it read exactly like every file that
// compiles.
//
// The second obligation is govet, and it exists because `make vet` runs
// untagged only. Switching govet off in .golangci.yml would take the vet
// analyzers away from every tagged file in the tree at once, and no other pass
// would report it.
//
// WHAT THIS CATCHES: a build tag anywhere in the repository that a golangci
// config does not carry, and govet disabled in either config.
//
// WHAT THIS DOES NOT CATCH, deliberately: whether carrying the tag is
// SUFFICIENT — a file can be linted and still be wrong, which is what the
// linters themselves are for. Nor does it judge GOOS/GOARCH constraints, which
// Go satisfies from the platform rather than from a -tags list; those are
// ratified in platformSelectedTags with what leaving them unread costs.

import (
	"go/build/constraint"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// treeRoot is the whole repository, deliberately wider than the trees golangci
// actually reads. A tag gating a file anywhere here is a tag whose files
// something ought to compile, and erring wide can only report a file that has
// no reader — never hide one.
const treeRoot = ".."

// platformSelectedTags names the constraints Go satisfies from GOOS/GOARCH
// rather than from a -tags list. No golangci entry could give these a reader,
// so each carries what leaving it out costs instead.
var platformSelectedTags = gatekit.Waive(map[string]string{
	"windows": "GOOS, so a build-tags entry would not reach it — golangci runs at the host GOOS. These files are read by the windows/amd64 cross-compile in `make build`, which is why that pass is in the always-on gate rather than the desktop lane",
	"darwin":  "GOOS, so a build-tags entry would not reach it. The cost is real and accepted: a darwin-only file is compiled on a developer's own machine and by nothing in the Linux merge gate, so it is gated locally or not at all",
})

func TestEveryBuildTagInTheTreeIsCarriedByBothLintConfigs(t *testing.T) {
	t.Parallel()
	defer platformSelectedTags.AssertAllMatched(t)

	found := buildTagsInTree(t)
	if len(found) == 0 {
		t.Fatal("the walk found no build constraint anywhere in the repository — a census that recognises " +
			"nothing reports PASS over every unread lane there is, so the walk is broken rather than the tree clean")
	}

	// Both configs, because they carry SEPARATE build-tags lists: a tag added
	// to one and not the other leaves half the linters blind, which is the
	// case hardest to see from either file on its own.
	for _, path := range lintConfigs {
		carried := readGolangciConfig(t, path).Run.BuildTags
		if len(carried) == 0 {
			t.Errorf("%s carries no build tag at all, so every tagged file in the tree is invisible to it", path)
			continue
		}
		for _, tag := range found {
			if platformSelectedTags.Waived(t, tag) || slices.Contains(carried, tag) {
				continue
			}
			t.Errorf("build tag %q gates a file in this tree but %s does not carry it, so nothing in the merge "+
				"gate compiles those files: add it to that config's run.build-tags, or ratify it in "+
				"platformSelectedTags with what leaving it unread costs", tag, path)
		}
	}
}

func TestBothLintConfigsRunGovet(t *testing.T) {
	t.Parallel()
	// `make vet` covers the untagged tree only, so these two passes are where
	// the vet analyzers reach a tagged file. Losing govet here is silent: the
	// lint pass still runs, still reports, and still passes.
	for _, path := range lintConfigs {
		cfg := readGolangciConfig(t, path)
		// golangci's `standard` and `all` sets both include govet; anything
		// else has to name it. `disable` wins over both, so it is asked last.
		enabled := cfg.Linters.Default == "standard" || cfg.Linters.Default == "all" ||
			slices.Contains(cfg.Linters.Enable, "govet")
		if !enabled || slices.Contains(cfg.Linters.Disable, "govet") {
			t.Errorf("%s does not run govet, so no pass in the merge gate vets a tagged file: `make vet` runs "+
				"untagged and stops there", path)
		}
	}
}

// buildTagsInTree returns every tag named by a build constraint under treeRoot,
// deduplicated and ordered.
//
// The tags are read out of the PARSED constraint rather than off the line,
// because the two disagree in exactly the direction that loses a tag: `//go:build
// integration && !race` names two, a line-splitter reports the operators as
// well, and a negated tag still names the tag it negates.
func buildTagsInTree(t *testing.T) []string {
	t.Helper()
	seen := map[string]bool{}
	err := filepath.WalkDir(treeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			// No hand-written Go in either, and both are thousands of files of
			// pure walk cost. node_modules is an installed dependency tree;
			// build/ is generated (gen-composition) and git-ignored, so it
			// holds a SECOND copy of files this walk has already counted —
			// present or absent depending on whether anyone has run `make
			// composition`, which would make the census non-deterministic.
			case "node_modules", "build", ".git":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(path) // #nosec G304 G122 -- path is a *.go file from walking the trusted source tree
		if err != nil {
			return err
		}
		// Constraints live above the package clause; both spellings are read
		// because Go still honours the legacy one, and a file carrying only
		// that would otherwise pass unseen.
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "package ") {
				break
			}
			if !constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line) {
				continue
			}
			expr, err := constraint.Parse(line)
			if err != nil {
				// An unparseable constraint is not a tag this gate can judge,
				// but it is also not something Go will build — reporting it
				// here beats skipping it silently.
				t.Errorf("%s: parsing build constraint %q: %v", filepath.ToSlash(path), line, err)
				continue
			}
			collectTags(expr, seen)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", treeRoot, err)
	}
	tags := make([]string, 0, len(seen))
	for tag := range seen {
		tags = append(tags, tag)
	}
	slices.Sort(tags)
	return tags
}

// collectTags walks a constraint structurally, which is what makes the census
// total. constraint.Expr.Eval would be the shorter spelling and the wrong one:
// it short-circuits, so the second operand of an `&&` whose first is already
// false is never visited and its tag never seen.
func collectTags(expr constraint.Expr, into map[string]bool) {
	switch e := expr.(type) {
	case *constraint.TagExpr:
		into[e.Tag] = true
	case *constraint.NotExpr:
		collectTags(e.X, into)
	case *constraint.AndExpr:
		collectTags(e.X, into)
		collectTags(e.Y, into)
	case *constraint.OrExpr:
		collectTags(e.X, into)
		collectTags(e.Y, into)
	}
}
