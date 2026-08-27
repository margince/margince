// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every Go file in this repository is compiled by a pass the merge gate runs,
// analysed by one that lints, and read by both lint configs or neither.
//
// A file is invisible to a pass whose build-tag assignment its constraint does
// not admit. Most of the tree is untagged and every pass takes it; the ones that
// matter are the tagged and platform-selected lanes, where a YAML list and a
// filename suffix decide whether anything reads them at all.
//
// A file whose tag no pass carries can call a function it does not import and
// never be told. Nothing type-checks it, and it reads on disk exactly like every
// file that compiles.
//
// THREE questions, none implying the others:
//
//   - COMPILED at all? A no means a type error in it never surfaces.
//   - ANALYSED by anything? Compiling is not analysing, and the distinction is
//     load-bearing: `make test-integration` compiles the tagged tree and runs
//     neither `go vet` nor golangci over it. A file only the shards admit is
//     built and read by no linter.
//   - Read by BOTH lint configs? They run different linter sets, so a file one
//     of them takes and the other does not keeps half its coverage silently.
//
// Whether a pass admits a file is asked of GO, not decided here. MatchFile is
// the same function the toolchain uses to build a package, so build tags, both
// constraint spellings, GOOS/GOARCH filename suffixes, `_test`, truncation at
// the first dot and the OS names that exist only for filenames (`zos`) are all
// answered by the implementation that owns them. A hand-rolled version of those
// rules was wrong three separate ways before it was replaced by this, which is
// the argument: the rules are Go's, Go revises them, and a copy is wrong in ways
// no reader of this file can see.
//
// WHAT THIS CATCHES: a file no pass compiles, one nothing lints, and one the two
// lint configs disagree about.
//
// WHAT THIS DOES NOT CATCH, deliberately: whether being analysed is ENOUGH — a
// file can be linted and still be wrong, which is what the linters are for.

import (
	"go/build"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

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

// compilingPass is one assignment the merge gate compiles this tree under.
//
// analyses marks the passes that also run a linter over what they compile. A
// pass that only builds still answers the type-error question, which is why it
// is counted at all.
type compilingPass struct {
	name     string
	goos     string
	tags     []string
	analyses bool
}

// admits reports whether this pass compiles the named file, asked of go/build so
// that no rule about constraints is restated here.
func (p compilingPass) admits(dir, name string) (bool, error) {
	ctx := build.Default
	ctx.GOOS = p.goos
	// The runner and every machine this gate runs on are 64-bit. A file
	// constrained to another architecture reports as unread, which is true of
	// these passes and worth being told.
	ctx.GOARCH = "amd64"
	ctx.BuildTags = p.tags
	return ctx.MatchFile(dir, name)
}

// mergeGatePasses are the assignments under which the merge gate compiles this
// tree. A file none of them admits is one nothing builds.
//
// The two golangci rows take their tags from the configs themselves, because
// that list is the one that drifts. The rest are named with the recipe they come
// from: their disappearance would be a deliberate edit to a compile step rather
// than a quiet YAML change.
//
// The integration shards are counted, and only as a COMPILING pass. A file only
// they admit is genuinely built — denying that would be the inverse error — but
// they run no linter over it.
func mergeGatePasses(t *testing.T) []compilingPass {
	t.Helper()
	passes := []compilingPass{
		{name: "untagged (`make build`, `make vet`, `make test`, and `make lint-modules` outside backend/)", goos: "linux", analyses: true},
		// `go build` never compiles a _test.go, so this row covers non-test
		// files only. It is listed anyway because a windows-only production
		// file would otherwise report as unread when that cross-compile does
		// read it; a windows-only TEST file is genuinely unread.
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

// unreadFiles ratifies a file the merge gate does not fully read, with what that
// costs. Every entry is platform-constrained, and that is not a coincidence:
// golangci runs at ONE GOOS, so a file selected by a different one cannot be
// linted by it at any tag setting. These are the files this repository builds for
// a platform its gate does not run on.
//
// The list is worth reading as a whole rather than entry by entry. It is the
// standing cost of shipping a desktop bundle from a Linux merge gate, it was
// invisible until the census learned to read filename constraints, and every
// line of it is code no linter has ever opened. The wider count, including the
// `//go:build !integration` files this gate cannot help with, is issue #2920.
var unreadFiles = gatekit.Waive(map[string]string{
	// Backend, cross-compiled for windows by `make build` and linted by nothing.
	"backend/internal/platform/blobstore/fs_sync_windows.go": "the windows half of a file-sync primitive: `make build` cross-compiles it so a type error is caught, but golangci runs at the host GOOS and never opens it — so gosec, depguard and revive have never read this syscall-adjacent code, and its unix sibling is the only half they judge",
	"backend/internal/platform/ownedfile/owned_windows.go":   "the windows half of the owned-file primitive, cross-compiled and unlinted for the reason its blobstore sibling above states — the cost is that a permissions or error-handling defect here is caught only by review",

	// The desktop launcher is its own module, released by its own lanes.
	"desktop/launcher/platform_windows.go":  "desktop launcher platform layer: built by the windows release lane (desktop-windows.yml), not by the merge gate, which neither cross-compiles this module nor lints at a foreign GOOS. A break here surfaces at release rather than at merge",
	"desktop/launcher/process_windows.go":   "desktop launcher process control, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"desktop/launcher/postgres_windows.go":  "desktop launcher Postgres supervision, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"desktop/launcher/runerror_windows.go":  "desktop launcher error rendering, built by the windows release lane alone — see platform_windows.go above for why no merge-gate pass reaches it",
	"desktop/launcher/browser_darwin.go":    "desktop launcher browser handoff for macOS: compiled by the macOS release lane (desktop-macos.yml) and by a maintainer's own machine, and by nothing at all in the merge gate — GOOS is not a tag, so no config entry could change that",
	"desktop/launcher/quarantine_darwin.go": "desktop launcher quarantine-attribute handling, macOS-only and reached only by the macOS release lane. This one carries the most risk of the set: it is the code that decides what a downloaded bundle is allowed to do, and no linter in this repository has read it",
})

func TestEveryGoFileIsCompiledAndAnalysedBySomePass(t *testing.T) {
	t.Parallel()
	defer unreadFiles.AssertAllMatched(t)

	passes := mergeGatePasses(t)
	names := make([]string, 0, len(passes))
	for _, p := range passes {
		names = append(names, p.name)
	}
	seen := 0
	forEachGoFile(t, func(rel, dir, name string) {
		seen++
		compiled, analysed := false, false
		for _, p := range passes {
			ok, err := p.admits(dir, name)
			if err != nil {
				t.Errorf("%s: asking go/build whether %q admits it: %v", rel, p.name, err)
				return
			}
			if !ok {
				continue
			}
			compiled = true
			analysed = analysed || p.analyses
		}
		switch {
		case compiled && analysed:
		case unreadFiles.Waived(t, rel):
		case compiled:
			t.Errorf("%s is compiled but ANALYSED by nothing: only a pass that builds without linting admits it, "+
				"so a type error in it surfaces and a vet, gosec or depguard finding never does. Add its tag to "+
				"both golangci configs, or ratify the file in unreadFiles with what leaving it unanalysed costs", rel)
		default:
			t.Errorf("%s is compiled by no pass the merge gate runs — none of %s admits it. Add its tag to both "+
				"golangci configs so `make lint` reads it, or ratify the file in unreadFiles with what leaving it "+
				"unread costs, which is the answer for `ignore`, the standard spelling for a file meant to be "+
				"`go run` and never built", rel, strings.Join(names, ", "))
		}
	})
	if seen == 0 {
		t.Fatal("the walk found no Go file at all — a census that recognises nothing reports PASS over every " +
			"unread lane there is, so the walk is broken rather than the tree clean")
	}
}

func TestBothLintConfigsReadTheSameFiles(t *testing.T) {
	t.Parallel()
	// The two configs run different linter sets: the baseline owns the
	// repo-wide gate (depguard's DAG, gosec, forbidigo, misspell), the strict
	// one owns the new-code rules. A file one takes and the other does not
	// keeps half its coverage, and the sibling census stays green because
	// something still analyses it.
	//
	// Asked per FILE rather than by comparing the two tag lists: a tag that
	// gates no file is not a divergence worth failing, and one that gates a
	// file under a negation is one that comparing lists would miss.
	var configs []compilingPass
	for _, path := range lintConfigs {
		configs = append(configs, compilingPass{name: path, goos: "linux", tags: readGolangciConfig(t, path).Run.BuildTags})
	}
	if len(configs) != 2 {
		t.Fatalf("expected two lint configs to compare, found %d", len(configs))
	}
	forEachGoFile(t, func(rel, dir, name string) {
		admits := [2]bool{}
		for i, cfg := range configs {
			ok, err := cfg.admits(dir, name)
			if err != nil {
				t.Errorf("%s: asking go/build whether %s admits it: %v", rel, cfg.name, err)
				return
			}
			admits[i] = ok
		}
		if admits[0] == admits[1] {
			return
		}
		reads, blind := configs[0].name, configs[1].name
		if admits[1] {
			reads, blind = blind, reads
		}
		t.Errorf("%s is read by %s and not by %s, so half this repository's linters do not see it: the two "+
			"configs' run.build-tags have diverged, and only one of them is the repo-wide gate", rel, reads, blind)
	})
}

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
		// here would take it from every tagged file in the tree at once.
		//
		// A ratified set rather than a copy of go vet's default analyzer list:
		// restating that list here would be a second copy of something the
		// toolchain owns and revises, and it would go stale silently.
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

// govetDisables ratifies each govet analyzer a config switches off, with what
// losing it costs across every tagged file.
var govetDisables = gatekit.Waive(map[string]string{
	"fieldalignment": "a memory-layout micro-optimisation, not a correctness check: it reports struct field ORDER, and acting on it trades legible grouping for bytes this product never counts",
	"shadow":         "high-noise on idiomatic Go — every `err :=` inside a block trips it — and the cases that are real bugs are reported by revive's early-return and by errcheck, so the coverage lost is the false-positive half",
})

// forEachGoFile visits every Go file in this repository, handing the callback the
// repository-relative path — what a waiver is keyed by and what a failure names,
// identical on every machine — alongside the directory and base name go/build
// needs.
func forEachGoFile(t *testing.T, visit func(rel, dir, name string)) {
	t.Helper()
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
			if slices.ContainsFunc(generatedTrees, func(tree string) bool {
				return filepath.Clean(path) == filepath.Clean(tree)
			}) {
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || !d.Type().IsRegular() {
			return nil
		}
		rel, err := filepath.Rel(repoRoot, path)
		if err != nil {
			return err
		}
		visit(filepath.ToSlash(rel), filepath.Dir(path), filepath.Base(path))
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", repoRoot, err)
	}
}
