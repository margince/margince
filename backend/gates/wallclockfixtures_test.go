// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H2

package gates

// The population of tests that date themselves from wall time may fall and
// never rise.
//
// A fixture built on time.Now() makes its claim about the day it runs. Seeded
// seven days out and asserted to group as `later`, it answers `this_week` on a
// Thursday. The day it changes its answer is a day nobody edited anything, so
// no pull-request gate can be the thing that finds it. What finds it is the
// drift lane, a second RUN of the suite at a moved clock.
//
// This gate is a ratchet rather than a census, and that is the whole design.
// Nothing static separates a fixture whose instant is compared against a
// boundary from one whose instant is merely stored. docs/reference/make-targets.md
// records the same finding for the frontend lane, where "an absolute date in a
// file that never pins the clock" matched 129 files, nearly all harmless. A gate
// reporting all of those would be noise, and noise is what teaches a reader to
// skip a census. So this one judges no site. It counts them, holds the count,
// and lets the lane do the judging.
//
// What this does not cover, so its silence is not read as a wider claim:
//   - Whether any counted site is actually fragile. It is a budget on a
//     population, and a file at its frozen count may still hold the next defect.
//   - time.Since and time.Until, which read the same clock. A COMPARISON
//     against an elapsed duration is mergegateclockbounds_test.go's subject and
//     is prohibited outright there; counting them here would report the same
//     line under two rules.
//   - A clock reached through an interface or a struct field rather than named.
//     Resolving that needs type information this gate does not load.
//   - A read moved OUT of a _test.go file into a build-tagged helper beside it,
//     the shape platform/testdb carries. The file's count drops to zero, this
//     gate then requires its ledger entry deleted, and nothing counts the read
//     again. No such helper holds one today. The alternative is to read every
//     .go file, which would report the whole product's legitimate use of the
//     clock: far more correct sites than wrong ones, and the noise that
//     teaches a reader to skip a census.
//   - A fixture spelling an absolute date (time.Date(2026, ...)), which is
//     calendar-fragile in the same way and which no count of time.Now
//     reaches. The lane sees it; this ledger does not.
//   - The 905 files whose SQL says now(). Those read the DATABASE's clock, and
//     only the machine applier moves it.

import (
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// ledgerPath records each test file that reads wall time, with the count frozen
// when it was admitted. A file may shrink; reaching zero means its entry comes
// OUT, so the file is held at zero for good.
const ledgerPath = "gates/testdata/wallclockfixtures.txt"

// clockOwners are the packages whose subject is the clock, so a wall-time read
// in their tests is the thing under test rather than a fixture dating itself.
var clockOwners = []string{
	"internal/platform/clocktest/",
	"internal/shared/clockskew/",
}

func TestTheWallClockFixturePopulationOnlyFalls(t *testing.T) {
	t.Parallel()

	frozen := readLedger(t)
	counted := countWallClockReads(t)

	for path, count := range counted {
		was, admitted := frozen[path]
		switch {
		case !admitted:
			t.Errorf("%s: %d wall-clock read(s) in a test file that had none. Date the fixture from "+
				"clocktest.Now(t) instead, so the drift lane can move it; if the instant genuinely "+
				"must be wall time, add the file to %s with its count and say why in the commit.",
				path, count, ledgerPath)
		case count > was:
			t.Errorf("%s: %d wall-clock read(s), up from the frozen %d. The ledger only falls — "+
				"new fixtures in an admitted file take their instant from clocktest.Now(t).",
				path, count, was)
		}
	}

	for path := range frozen {
		if counted[path] > 0 {
			continue
		}
		if _, err := os.Stat(path); err != nil {
			t.Errorf("%s: %s names a file that no longer exists — a stale entry hides the next "+
				"file that takes its place.", ledgerPath, path)
			continue
		}
		t.Errorf("%s: %s now reads no wall clock. Remove its entry, which is what holds it at zero.",
			ledgerPath, path)
	}
}

// countWallClockReads returns every backend test file holding at least one
// time.Now() call, with how many.
//
// Read with the language's parser rather than by scanning text: a scanner that
// runs off the end of a string goes blind and reports OK over the rest of the
// file, which is the one failure a population budget must not have. A parse
// error is the answer here, not an inconvenience.
func countWallClockReads(t *testing.T) map[string]int {
	t.Helper()

	counted := map[string]int{}
	// TestMain has left the working directory at the module root.
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return err
		}
		rel := filepath.ToSlash(path)
		for _, owner := range clockOwners {
			if strings.HasPrefix(rel, owner) {
				return nil
			}
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Errorf("parsing %s: %v", rel, err)
			return nil
		}
		if n := wallClockReads(file); n > 0 {
			counted[rel] = n
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if len(counted) == 0 {
		t.Fatal("no wall-clock read found anywhere in the backend's tests — the walk examined " +
			"nothing, which is not the same as finding nothing. Retarget it.")
	}
	return counted
}

// wallClockReads counts one parsed file's reads of the wall clock.
//
// A read, not a call. `NewStore(db, time.Now)` hands the clock over as a value
// and is the dominant injection idiom here: the store stamps rows at wall time
// and no fixture helper can move it, which makes those the sites that matter
// most.
//
// The package is resolved through the file's IMPORTS rather than matched on the
// spelling `time`. Go lets a file bind the package to any name (`import
// walltime "time"` makes every read `walltime.Now()`), and a counter keyed on
// the word would return zero for such a file, admit it as reading no clock, and
// report PASS. That is under-recognition, the one direction a census must not
// fail in: it reads a smaller tree and there is no failing assertion to notice.
//
// It can over-count, and that is the safe direction. A local variable named
// `time` with a Now method would be counted, because resolving that needs type
// information this gate does not load; the result is a file that appears to
// hold more reads than it does, which fails loudly and is fixed by a waiver
// line rather than passing in silence.
func wallClockReads(file *ast.File) int {
	names, dotted := timePackageNames(file)
	found := 0
	ast.Inspect(file, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		if pkg, ok := selector.X.(*ast.Ident); ok && selector.Sel.Name == "Now" && names[pkg.Name] {
			found++
		}
		return true
	})
	if dotted {
		found += dotImportedNowReads(file)
	}
	return found
}

// dotImportedNowReads counts the bare Now a dot import puts in the file's own
// scope. Reached only for a file that dot-imported time: an unqualified `Now`
// is not distinctive on its own (a method, a field and a local can all wear
// the name), so matching it anywhere else would report far more correct sites
// than wrong ones.
func dotImportedNowReads(file *ast.File) int {
	found := 0
	var count func(n ast.Node) bool
	count = func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.SelectorExpr:
			// Only the operand of a qualified name can hold a bare Now; its Sel
			// is somebody's field or method, and a package-qualified read was
			// already counted by the caller.
			ast.Inspect(node.X, count)
			return false
		case *ast.Ident:
			if node.Name == "Now" {
				found++
			}
		}
		return true
	}
	ast.Inspect(file, count)
	return found
}

// timePackageNames returns every local name this file can call the time package
// by, and whether it dot-imported it.
//
// A blank import binds no name and is therefore absent from both: it cannot
// carry a call.
func timePackageNames(file *ast.File) (names map[string]bool, dotted bool) {
	names = map[string]bool{}
	for _, imported := range file.Imports {
		path, err := strconv.Unquote(imported.Path.Value)
		if err != nil || path != "time" {
			continue
		}
		switch {
		case imported.Name == nil:
			names["time"] = true
		case imported.Name.Name == ".":
			dotted = true
		case imported.Name.Name == "_":
		default:
			names[imported.Name.Name] = true
		}
	}
	return names, dotted
}

// readLedger parses the frozen counts. A malformed line fails rather than being
// skipped: a budget that silently ignored an entry it could not read would
// admit whatever that entry was holding.
func readLedger(t *testing.T) map[string]int {
	t.Helper()

	text, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatalf("reading %s: %v", ledgerPath, err)
	}
	frozen := map[string]int{}
	for i, line := range strings.Split(string(text), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		path, count, ok := strings.Cut(line, " ")
		n, err := strconv.Atoi(strings.TrimSpace(count))
		if !ok || err != nil || n <= 0 {
			t.Fatalf("%s:%d: %q is not `<path> <count>` with a positive count", ledgerPath, i+1, line)
		}
		// A second line for one path would otherwise overwrite the first, so
		// the higher of two counts silently becomes the ceiling: the ledger
		// raised by an edit that looks like an addition.
		if was, seen := frozen[path]; seen {
			t.Fatalf("%s:%d: %s is listed twice, frozen at %d and again at %d. One line per file, or "+
				"the larger entry quietly becomes the budget.", ledgerPath, i+1, path, was, n)
		}
		frozen[path] = n
	}
	return frozen
}
