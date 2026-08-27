// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every Go file in this repository is compiled by at least one pass the merge
// gate runs, and both lint configs run govet.
//
// A file is invisible to a pass whose build-tag assignment its constraint does
// not admit. Most of the tree is untagged and every pass compiles it; the ones
// that matter are the tagged lanes, where a single YAML list decides whether
// anything reads them at all.
//
// That is not a hypothetical failure mode. It is how a file reached this tree
// calling a function it does not import: no pass carried its tag, so nothing
// ever type-checked it, and it read exactly like every file that compiles.
//
// The subject is the CONSTRAINT, not the tags in it. Asking only whether each
// tag appears somewhere is a weaker question that passes on the case worth
// catching: `//go:build integration && !bench` names two tags both lint configs
// carry, and is compiled by neither — because those configs set `bench` too,
// which makes the second half false. Tag-presence says covered; nothing reads
// it. So each constraint is EVALUATED against each pass's assignment, which is
// also the only reading that gets negation and `||` right.
//
// The second obligation is govet, and it exists because `make vet` runs
// untagged only. Switching govet off in .golangci.yml — or leaving it enabled
// with every analyzer disabled, which reads as enabled to anything that looks
// only at the linter list — takes the vet analyzers away from every tagged file
// in the tree at once, and no other pass would report it.
//
// WHAT THIS CATCHES: a build constraint no pass in the merge gate satisfies,
// and govet reduced to nothing in either config.
//
// WHAT THIS DOES NOT CATCH, deliberately: whether being compiled is ENOUGH — a
// file can be linted and still be wrong, which is what the linters are for.

import (
	"go/build/constraint"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// treeRoot is the whole repository, deliberately wider than the trees the lint
// passes read. A constraint gating a file anywhere here is one something ought
// to compile, and erring wide can only report a file that has no reader — never
// hide one.
const treeRoot = ".."

// generatedTree is skipped by PATH rather than by directory name. `build` is an
// ordinary package name, and skipping every directory called that would drop a
// real one — `internal/build/…` — out of the census silently, which is the
// direction a census must never fail in. Only the repository's own generated
// composition tree is meant: it is git-ignored and holds a second copy of files
// already counted, so including it makes the census depend on whether anyone
// has run `make composition`.
var generatedTree = filepath.Join(treeRoot, "build")

// compilingPass is one assignment: the build tags set, and the GOOS the pass
// runs at. GOOS matters because Go satisfies a `//go:build windows` constraint
// from the target platform and never from a -tags list, so a pass model without
// it reports the cross-compiled files as unread.
type compilingPass struct {
	name string
	goos string
	tags []string
}

// satisfies reports whether this pass compiles a file carrying expr.
//
// Eval short-circuits, which is correct HERE and wrong for collecting tag
// names: the question is the expression's value under one assignment, and an
// operand that cannot change the answer genuinely does not need visiting.
func (p compilingPass) satisfies(expr constraint.Expr) bool {
	return expr.Eval(func(tag string) bool {
		switch {
		case slices.Contains(p.tags, tag):
			return true
		case tag == p.goos:
			return true
		// The runner and every machine this repo is built on are 64-bit; a
		// constraint naming another architecture reports as unread, which is
		// true of these passes and worth being told.
		case tag == "amd64" || tag == "arm64":
			return true
		// Go defines `unix` for the platforms it names, and the toolchain
		// satisfies its own release tags. Neither can be spelled in a -tags
		// list, so neither is a coverage question.
		case tag == "unix":
			return p.goos == "linux" || p.goos == "darwin"
		case strings.HasPrefix(tag, "go1."):
			return true
		default:
			return false
		}
	})
}

// mergeGatePasses are the assignments under which the merge gate compiles this
// tree. A constraint satisfied by none of them describes a file nothing reads.
//
// The two golangci rows take their tags from the configs themselves, because
// that list is the one that drifts. The rest are named with the recipe they
// come from: their disappearance would be a deliberate edit to a compile step
// rather than a quiet YAML change.
//
// The integration shards are counted. A file only they compile IS compiled, and
// a gate that pretended otherwise would deny a reader that exists — the inverse
// error, and just as misleading.
func mergeGatePasses(t *testing.T) []compilingPass {
	t.Helper()
	passes := []compilingPass{
		{name: "untagged (`make build`, `make vet`, `make test`)", goos: "linux"},
		{name: "the windows/amd64 cross-compile (`make build`)", goos: "windows"},
		{name: "the integration shards (`make test-integration`)", goos: "linux", tags: []string{"integration"}},
	}
	for _, path := range lintConfigs {
		carried := readGolangciConfig(t, path).Run.BuildTags
		if len(carried) == 0 {
			t.Errorf("%s carries no build tag at all, so every tagged file in the tree is invisible to it", path)
		}
		passes = append(passes, compilingPass{name: path + " (`make lint`)", goos: "linux", tags: carried})
	}
	return passes
}

// unreadFiles ratifies a file whose constraint no pass satisfies and which is
// nonetheless not a defect, with what leaving it unread costs.
var unreadFiles = gatekit.Waive(map[string]string{})

func TestEveryBuildConstraintIsCompiledBySomePass(t *testing.T) {
	t.Parallel()
	defer unreadFiles.AssertAllMatched(t)

	passes := mergeGatePasses(t)
	constrained := constrainedFiles(t)
	if len(constrained) == 0 {
		t.Fatal("the walk found no build constraint anywhere in the repository — a census that recognises " +
			"nothing reports PASS over every unread lane there is, so the walk is broken rather than the tree clean")
	}

	names := make([]string, 0, len(passes))
	for _, p := range passes {
		names = append(names, p.name)
	}
	for _, f := range constrained {
		if slices.ContainsFunc(passes, func(p compilingPass) bool { return p.satisfies(f.expr) }) {
			continue
		}
		if unreadFiles.Waived(t, f.path) {
			continue
		}
		t.Errorf("%s is compiled by no pass the merge gate runs: its constraint `%s` is false under every one of "+
			"%s. Add the tag to both golangci configs so `make lint` reads it, or ratify the file in unreadFiles "+
			"with what leaving it unread costs", f.path, f.expr.String(), strings.Join(names, ", "))
	}
}

// TestEveryTagIsCarriedByBothLintConfigs is the SECOND obligation, and it is
// not implied by the first.
//
// Being compiled by some pass is enough to be type-checked and no more. The two
// configs run different linter sets — the baseline owns the repo-wide gate
// (depguard's DAG, gosec, forbidigo, misspell), the strict one owns the
// new-code rules — so a tag carried by one and not the other leaves half of
// them blind on those files while the constraint census stays green, because
// the other config still compiles them.
//
// It is stated separately rather than folded in because the two fail for
// different reasons and want different fixes: this one is always "add the tag
// to the other config", never "ratify the file".
func TestEveryTagIsCarriedByBothLintConfigs(t *testing.T) {
	t.Parallel()
	defer platformSelectedTags.AssertAllMatched(t)

	tags := map[string]bool{}
	for _, f := range constrainedFiles(t) {
		collectTags(f.expr, tags)
	}
	if len(tags) == 0 {
		t.Fatal("the walk found no build constraint anywhere in the repository — a census that recognises " +
			"nothing reports PASS over every unread lane there is, so the walk is broken rather than the tree clean")
	}
	for _, path := range lintConfigs {
		carried := readGolangciConfig(t, path).Run.BuildTags
		for _, tag := range slices.Sorted(maps.Keys(tags)) {
			if platformSelectedTags.Waived(t, tag) || slices.Contains(carried, tag) {
				continue
			}
			t.Errorf("build tag %q gates a file in this tree but %s does not carry it, so that config's linters "+
				"do not read those files even where another pass compiles them: add it to that config's "+
				"run.build-tags, or ratify it in platformSelectedTags with what leaving it out costs", tag, path)
		}
	}
}

// platformSelectedTags names the constraints Go satisfies from GOOS/GOARCH
// rather than from a -tags list. No golangci entry could carry them, so each
// says what that costs instead.
var platformSelectedTags = gatekit.Waive(map[string]string{
	"windows": "GOOS, so a build-tags entry would not reach it — golangci runs at the host GOOS. The windows-only files are read by the windows/amd64 cross-compile in `make build`, which is why that pass is in the always-on gate rather than the desktop lane",
	"darwin":  "GOOS, so a build-tags entry would not reach it. Every darwin mention in this tree today is a NEGATION (`!darwin`), which the Linux passes satisfy — a darwin-ONLY file would be compiled on a maintainer's machine and by nothing in the merge gate, and this entry is what stops that reading as covered",
})

// collectTags walks a constraint structurally, which is what makes this census
// total. constraint.Expr.Eval would be the shorter spelling and the wrong one
// HERE: it short-circuits, so the second operand of an `&&` whose first is
// already false is never visited and its tag never seen. (Eval is right in
// compilingPass.satisfies, where the question is the expression's VALUE.)
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
			continue
		}
		// Enabled and then emptied is the same outcome reached by a different
		// door, and it reads as enabled to anything that checks only the linter
		// list. `disable-all` alongside an explicit `enable` set is a deliberate
		// narrowing the config may make; alongside nothing it leaves govet
		// running zero analyzers.
		govet := cfg.Linters.Settings.Govet
		if govet.DisableAll && len(govet.Enable) == 0 {
			t.Errorf("%s enables govet and then disables every analyzer it has, so the vet suite reaches no "+
				"tagged file: drop linters.settings.govet.disable-all, or name the analyzers to keep under "+
				"linters.settings.govet.enable", path)
		}
	}
}

// constrainedFile is one file that carries a build constraint, and the
// constraint it carries.
type constrainedFile struct {
	path string
	expr constraint.Expr
}

// constrainedFiles returns every file under treeRoot whose header carries a
// build constraint, paired with the PARSED constraint — the constraint is what
// this gate judges, and reducing it to the tags it names loses the negation and
// the operators that decide which passes admit the file.
func constrainedFiles(t *testing.T) []constrainedFile {
	t.Helper()
	var found []constrainedFile
	err := filepath.WalkDir(treeRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// node_modules and .git hold no hand-written Go and are thousands
			// of files of pure walk cost; the generated tree is skipped by
			// path, for the reason stated on generatedTree.
			if d.Name() == "node_modules" || d.Name() == ".git" || filepath.Clean(path) == filepath.Clean(generatedTree) {
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
				// An unparseable constraint is not one this gate can judge, but
				// it is also not something Go will build — reporting it here
				// beats skipping it silently.
				t.Errorf("%s: parsing build constraint %q: %v", filepath.ToSlash(path), line, err)
				continue
			}
			found = append(found, constrainedFile{path: filepath.ToSlash(path), expr: expr})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", treeRoot, err)
	}
	return found
}
