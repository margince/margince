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
// A file whose tag no pass carries can call a function it does not import and
// never be told. Nothing type-checks it, and it reads on disk exactly like every
// file that compiles.
//
// THREE questions are asked, and none implies the others.
//
// Is the file COMPILED at all? A no means a type error in it never surfaces.
// This is about the whole CONSTRAINT rather than the tags in it: `bench &&
// !integration` names two tags both lint configs carry and is admitted by no
// pass, because those configs set `integration` too and the integration lane
// does not set `bench`. So each constraint is EVALUATED against each pass's
// assignment, which is also the only reading that gets negation and `||` right.
//
// Is it ANALYSED by anything? Compiling is not analysing, and the distinction is
// load-bearing rather than pedantic: `make test-integration` compiles the tagged
// tree and runs neither `go vet` nor golangci over it. So `integration &&
// !bench` is compiled and read by no linter — a file the shards build and vet
// never sees. Splitting the two is what makes that fail here rather than pass.
//
// Is it linted by BOTH configs? That one IS about the tags, because the two
// configs run different linter sets and a file only one of them reads keeps half
// the linters. Equal-satisfaction between the two configs would be the tidier
// spelling and a weaker one: it agrees when a tag is missing from both, which is
// exactly how 853 integration-tagged files could lose every linter at once.
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
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// skipForeignTree reports whether a directory holds something other than this
// repository's own source: dependencies somebody else wrote, a nested checkout,
// or a tool's output. Every gate that walks this tree asks the same question, so
// it is asked in one place — a second list drifts, and the half that goes stale
// is the half that silently widens or narrows a census.
//
// Worktrees are the entry that earns this: this repository is worked in several
// at once by design, and one placed inside the tree is a whole second checkout
// whose files answer no question about this branch.
func skipForeignTree(name string) bool {
	switch name {
	case "node_modules", ".git", ".tmp", "worktrees", ".claude", "dist", "coverage":
		return true
	}
	return false
}

// generatedTrees are skipped by PATH rather than by directory name. `build` is
// an ordinary package name, and skipping every directory called that would drop
// a real package out of the census silently — the direction a census must never
// fail in.
//
// These three and no more, matching what .gitignore anchors: it names them
// individually and says why, so that an unrelated future /build asset is never
// hidden. A name-based skip here would hide precisely what that anchoring went
// out of its way to keep visible. Each holds generated copies of files already
// counted, present or absent depending on whether anyone has run the generator.
var generatedTrees = []string{
	filepath.Join(repoRoot, "build", "composition"),
	filepath.Join(repoRoot, "build", "composition-frontend"),
	filepath.Join(repoRoot, "build", "desktop"),
}

// compilingPass is one assignment: the build tags set, and the GOOS the pass
// runs at. GOOS matters because Go satisfies a `//go:build windows` constraint
// from the target platform and never from a -tags list, so a pass model without
// it reports the cross-compiled files as unread.
type compilingPass struct {
	name string
	goos string
	tags []string
	// analyses reports whether this pass runs a linter over what it compiles.
	// A pass that only builds still answers the type-error question, which is
	// why it is counted at all.
	analyses bool
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
// tree. A constraint satisfied by none of them describes a file nothing builds.
//
// `analyses` marks the passes that also run a linter over what they compile.
// The integration shards do not: they build the tagged tree to run its tests,
// which surfaces a type error and no vet finding.
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
		{name: "untagged (`make build`, `make vet`, `make test`, and `make lint-modules` over every module outside backend/)", goos: "linux", analyses: true},
		// `go build` never compiles a _test.go, so this row covers non-test
		// files only. It is listed anyway because a windows-only production
		// file would otherwise report as unread when that cross-compile does
		// read it; a windows-only TEST file is genuinely unread, and the
		// narrower claim is the one this row is allowed to make.
		{name: "the windows/amd64 cross-compile of non-test files (`make build`)", goos: "windows"},
		{name: "the integration shards (`make test-integration`)", goos: "linux", tags: []string{"integration"}},
	}
	for _, path := range lintConfigs {
		carried := readGolangciConfig(t, path).Run.BuildTags
		if len(carried) == 0 {
			t.Errorf("%s carries no build tag at all, so every tagged file in the tree is invisible to it", path)
		}
		passes = append(passes, compilingPass{name: path + " (`make lint`)", goos: "linux", tags: carried, analyses: true})
	}
	return passes
}

// unreadFiles ratifies a file the merge gate does not read, with what that
// costs. Every entry here is platform-constrained, and that is not a
// coincidence: golangci runs at ONE GOOS, so a file selected by a different one
// cannot be linted by it at any tag setting. These are the files this
// repository builds for a platform its gate does not run on.
//
// The list is worth reading as a whole rather than entry by entry. It is the
// standing cost of shipping a desktop bundle from a Linux merge gate, it was
// invisible until the census learned to read filename constraints, and every
// line of it is code no linter has ever opened.
var unreadFiles = gatekit.Waive(map[string]string{
	// Backend, compiled for windows by `make build` and linted by nothing.
	"../backend/internal/platform/blobstore/fs_sync_windows.go": "the windows half of a file-sync primitive: `make build` cross-compiles it so a type error is caught, but golangci runs at the host GOOS and never opens it — so gosec, depguard and revive have never read this syscall-adjacent code, and its unix sibling is the only half they judge",
	"../backend/internal/platform/ownedfile/owned_windows.go":   "the windows half of the owned-file primitive, cross-compiled and unlinted for the reason its blobstore sibling above states — the cost is that a permissions or error-handling defect here is caught only by review",

	// The desktop launcher is its own module, released by its own lanes.
	"../desktop/launcher/platform_windows.go":  "desktop launcher platform layer: built by the windows release lane (desktop-windows.yml), not by the merge gate, which neither cross-compiles this module nor lints at a foreign GOOS. A break here surfaces at release rather than at merge",
	"../desktop/launcher/process_windows.go":   "desktop launcher process control, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"../desktop/launcher/postgres_windows.go":  "desktop launcher Postgres supervision, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"../desktop/launcher/runerror_windows.go":  "desktop launcher error rendering, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"../desktop/launcher/browser_darwin.go":    "desktop launcher browser handoff for macOS: compiled by the macOS release lane (desktop-macos.yml) and by a maintainer's own machine, and by nothing at all in the merge gate — GOOS is not a tag, so no config entry could change that",
	"../desktop/launcher/quarantine_darwin.go": "desktop launcher quarantine-attribute handling, macOS-only and reached only by the macOS release lane. This one carries the most risk of the set: it is the code that decides what a downloaded bundle is allowed to do, and no linter in this repository has read it",
})

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
	analysing := make([]compilingPass, 0, len(passes))
	for _, p := range passes {
		names = append(names, p.name)
		if p.analyses {
			analysing = append(analysing, p)
		}
	}
	for _, f := range constrained {
		compiled := slices.ContainsFunc(passes, func(p compilingPass) bool { return p.satisfies(f.expr) })
		read := slices.ContainsFunc(analysing, func(p compilingPass) bool { return p.satisfies(f.expr) })
		if compiled && read {
			continue
		}
		if unreadFiles.Waived(t, f.path) {
			continue
		}
		if compiled {
			t.Errorf("%s is compiled but ANALYSED by nothing: its constraint `%s` is satisfied only by a pass "+
				"that builds without linting, so a type error in it surfaces and a vet, gosec or depguard finding "+
				"never does. Add the tag to both golangci configs, or ratify the file in unreadFiles with what "+
				"leaving it unanalysed costs", f.path, f.expr.String())
			continue
		}
		t.Errorf("%s is compiled by no pass the merge gate runs: its constraint `%s` is false under every one of "+
			"%s. Add the tag to both golangci configs so `make lint` reads it, or ratify the file in unreadFiles "+
			"with what leaving it unread costs — which is the answer for `ignore`, the standard spelling for a "+
			"file meant to be `go run` and never built", f.path, f.expr.String(), strings.Join(names, ", "))
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
	"windows": "GOOS, so a build-tags entry would not reach it — golangci runs at the host GOOS. Every windows mention in this tree today is a NEGATION (`!windows`), which the Linux passes satisfy; a windows-ONLY file would be read by the `make build` cross-compile if it were production code and by nothing at all if it were a test, since `go build` skips _test.go",
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

// govetDisables ratifies each govet analyzer a config switches off, with what
// losing it costs across every tagged file.
var govetDisables = gatekit.Waive(map[string]string{
	"fieldalignment": "a memory-layout micro-optimisation, not a correctness check: it reports struct field ORDER, and acting on it trades legible grouping for bytes this product never counts",
	"shadow":         "high-noise on idiomatic Go — every `err :=` inside a block trips it — and the cases that are real bugs are reported by revive's early-return and by errcheck, so the coverage lost is the false-positive half",
})

func TestBothLintConfigsRunGovet(t *testing.T) {
	t.Parallel()
	defer govetDisables.AssertAllMatched(t)
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
		// Enabled and then emptied is the same outcome by a different door, and
		// it reads as enabled to anything that checks only the linter list.
		// `disable-all` alongside an explicit `enable` set is a deliberate
		// narrowing the config may make; alongside nothing it leaves govet
		// running zero analyzers.
		govet := cfg.Linters.Settings.Govet
		if govet.DisableAll && len(govet.Enable) == 0 {
			t.Errorf("%s enables govet and then disables every analyzer it has, so the vet suite reaches no "+
				"tagged file: drop linters.settings.govet.disable-all, or name the analyzers to keep under "+
				"linters.settings.govet.enable", path)
		}
		// The narrower door, and the one already ajar: `disable` removes named
		// analyzers while the linter still reports as enabled. Deleting `printf`
		// here would take it from every tagged file in the tree at once, which
		// is the coverage the removed `go vet -tags` pass used to backstop.
		//
		// The check is a RATIFIED set rather than a copy of go vet's default
		// analyzer list. Restating that list here would be a second copy of
		// something the toolchain owns and revises, and it would go stale
		// silently; requiring a reason per entry costs one line and says why
		// each absence is affordable.
		for _, analyzer := range govet.Disable {
			if govetDisables.Waived(t, analyzer) {
				continue
			}
			t.Errorf("%s disables the govet analyzer %q, and `make vet` runs untagged only — so that check is "+
				"gone from every tagged file in the tree. Ratify it in govetDisables with what losing it costs, "+
				"or stop disabling it", path, analyzer)
		}
	}
}

// andConstraints joins two constraints the way Go does when a file carries both
// a header and a filename suffix. Either side may be absent.
func andConstraints(a, b constraint.Expr) constraint.Expr {
	switch {
	case a == nil:
		return b
	case b == nil:
		return a
	default:
		return &constraint.AndExpr{X: a, Y: b}
	}
}

// platformNames holds the GOOS and GOARCH names SEPARATELY, because Go's
// filename suffixes are ordered and typed rather than "any two platform words".
type platformNames struct {
	goos   map[string]bool
	goarch map[string]bool
}

// knownPlatforms asks the TOOLCHAIN for those names rather than writing them
// down here.
//
// A literal list would be a second copy of something the Go release owns and
// revises, and it would go stale in the silent direction: a suffix this gate did
// not recognise is a file it reads as unconstrained, which is the census going
// short again in exactly the place this function exists to fix.
func knownPlatforms(t *testing.T) platformNames {
	t.Helper()
	out, err := exec.Command("go", "tool", "dist", "list").Output()
	if err != nil {
		t.Fatalf("asking the toolchain for its GOOS/GOARCH names: %v", err)
	}
	known := platformNames{goos: map[string]bool{}, goarch: map[string]bool{}}
	for _, pair := range strings.Fields(string(out)) {
		goos, goarch, found := strings.Cut(pair, "/")
		if !found {
			t.Fatalf("`go tool dist list` printed %q, which is not a GOOS/GOARCH pair", pair)
		}
		known.goos[goos] = true
		known.goarch[goarch] = true
	}
	if len(known.goos) == 0 || len(known.goarch) == 0 {
		t.Fatal("the toolchain named no platform at all, so every filename suffix below reads as unconstrained")
	}
	return known
}

// filenameConstraint returns the constraint Go applies to a file for its NAME
// alone — `x_windows.go`, `x_amd64.go`, `x_windows_amd64.go` — or nil.
//
// The two suffix positions are TYPED and ORDERED, not "any two platform words",
// and reading them as interchangeable produces constraints Go never forms.
// `x_linux_windows.go` is a windows file: `windows` is not a GOARCH, so Go takes
// it as the GOOS and `linux` is part of the name. Treating both as platforms
// yields `linux && windows`, which nothing satisfies — the gate would report a
// file that builds fine as read by nothing. Over-recognition is the safer
// direction to fail in and still the wrong answer.
//
// `_test` is stripped first, so `x_windows_test.go` is constrained exactly as
// `x_windows.go` is. The first segment is the file's own name and is never read
// as a platform, which is what stops a file called `windows.go` counting.
func filenameConstraint(name string, platforms platformNames) constraint.Expr {
	base := strings.TrimSuffix(name, ".go")
	base = strings.TrimSuffix(base, "_test")
	parts := strings.Split(base, "_")
	var goos, goarch string
	switch n := len(parts); {
	case n >= 2 && platforms.goarch[parts[n-1]]:
		goarch = parts[n-1]
		if n >= 3 && platforms.goos[parts[n-2]] {
			goos = parts[n-2]
		}
	case n >= 2 && platforms.goos[parts[n-1]]:
		goos = parts[n-1]
	}
	var expr constraint.Expr
	for _, tag := range []string{goos, goarch} {
		if tag != "" {
			expr = andConstraints(expr, &constraint.TagExpr{Tag: tag})
		}
	}
	return expr
}

// constrainedFile is one file that carries a build constraint, and the
// constraint it carries.
type constrainedFile struct {
	path string
	expr constraint.Expr
}

// constrainedFiles returns every file under repoRoot that Go constrains at all,
// paired with the PARSED constraint — the constraint is what this gate judges,
// and reducing it to the tags it names loses the negation and the operators that
// decide which passes admit the file.
//
// BOTH sources of constraint, because Go honours both and a census that reads
// one is short by exactly the files that use the other. A `//go:build` header is
// the visible spelling; `_windows.go`, `_amd64.go` and `_windows_amd64.go`
// constrain by NAME with nothing in the file to see. This tree has eight such
// files carrying no header at all, and the first version of this gate did not
// know they existed. Where a file has both, the two are ANDed, which is what Go
// does.
func constrainedFiles(t *testing.T) []constrainedFile {
	t.Helper()
	platforms := knownPlatforms(t)
	var found []constrainedFile
	err := filepath.WalkDir(repoRoot, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			// Foreign trees first — a linked worktree under .claude/ or a
			// scratch file under .tmp/ is ANOTHER checkout's source, and
			// reporting one of its files here prints a remedy that cannot be
			// followed because the file is not in this tree. The generated
			// trees are then skipped by path, for the reason on generatedTrees.
			if skipForeignTree(d.Name()) {
				return fs.SkipDir
			}
			if slices.ContainsFunc(generatedTrees, func(t string) bool {
				return filepath.Clean(path) == filepath.Clean(t)
			}) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !d.Type().IsRegular() {
			return nil
		}
		// path comes from walking this repository's own source tree, never from
		// input. (No #nosec token: both configs exclude gosec on _test.go, so
		// one here would suppress a linter that does not run.)
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Constraints live above the package clause; both spellings are read
		// because Go still honours the legacy one, and a file carrying only
		// that would otherwise pass unseen.
		var expr constraint.Expr
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(strings.TrimSpace(line), "package ") {
				break
			}
			if !constraint.IsGoBuild(line) && !constraint.IsPlusBuild(line) {
				continue
			}
			parsed, err := constraint.Parse(line)
			if err != nil {
				// An unparseable constraint is not one this gate can judge, but
				// it is also not something Go will build — reporting it here
				// beats skipping it silently.
				t.Errorf("%s: parsing build constraint %q: %v", filepath.ToSlash(path), line, err)
				continue
			}
			expr = andConstraints(expr, parsed)
		}
		expr = andConstraints(expr, filenameConstraint(filepath.Base(path), platforms))
		if expr != nil {
			found = append(found, constrainedFile{path: filepath.ToSlash(path), expr: expr})
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", repoRoot, err)
	}
	return found
}
