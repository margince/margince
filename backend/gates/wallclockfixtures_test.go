// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H2

package gates

// The population of tests that date themselves from wall time may fall and
// never rise.
//
// A fixture built on time.Now() makes its claim about the day it runs, so the
// day it changes its answer is a day nobody edited anything. The drift lane, a
// second run of the suite at a moved clock, is what finds it. This gate judges
// no site, because nothing static separates an instant compared against a
// boundary from one merely stored; it holds the count and lets the lane judge.
//
// What this does not cover, so its silence is not read as a wider claim:
//   - Whether any counted site is fragile. A file at its frozen count may
//     still hold the next defect.
//   - time.Since and time.Until. Comparing an elapsed duration is
//     mergegateclockbounds_test.go's subject and is prohibited outright there.
//   - A clock reached through an interface or a struct field, which needs type
//     information this gate does not load.
//   - A read moved out of a _test.go file into a build-tagged helper beside
//     it. Reading every .go file instead would report the product's legitimate
//     use of the clock, far more correct sites than wrong ones.
//   - An absolute date (time.Date(2026, ...)), which the lane sees and no count
//     of time.Now reaches.
//   - SQL that says now(). That is the database's clock, and only the machine
//     applier moves it.

import (
	"fmt"
	"go/ast"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// ledgerPath records each test file that reads wall time, with its current
// count. A file may shrink and its entry follows it down; at zero the entry
// comes out, which holds the file at zero for good.
const ledgerPath = "gates/testdata/wallclockfixtures.txt"

// clockOwners are the packages whose subject is the clock, so a wall-time read
// in their tests is the thing under test rather than a fixture dating itself.
var clockOwners = []string{
	"internal/platform/clocktest/",
	"internal/shared/clockskew/",
}

func TestTheWallClockFixturePopulationOnlyFalls(t *testing.T) {
	t.Parallel()

	for _, finding := range ledgerDrift(readLedger(t), countWallClockReads(t), fileExists) {
		t.Error(finding)
	}
}

// TestTheWallClockLedgerAnswersEveryPlantedShape plants each way the tree can
// disagree with the ledger, so a comparison that stops seeing one fails here
// rather than reporting PASS over the real tree.
func TestTheWallClockLedgerAnswersEveryPlantedShape(t *testing.T) {
	t.Parallel()

	present := func(string) bool { return true }
	cases := []struct {
		name    string
		frozen  map[string]int
		counted map[string]int
		exists  func(string) bool
		want    string
	}{
		{"an unlisted file", map[string]int{}, map[string]int{"a_test.go": 1}, present, "had none"},
		{"a count that rose", map[string]int{"a_test.go": 2}, map[string]int{"a_test.go": 3}, present, "up from the frozen 2"},
		{"a count that fell", map[string]int{"a_test.go": 5}, map[string]int{"a_test.go": 2}, present, "lower the frozen count to 2"},
		{"a file down to zero", map[string]int{"a_test.go": 1}, map[string]int{}, present, "Remove its entry"},
		{"a deleted file", map[string]int{"a_test.go": 1}, map[string]int{}, func(string) bool { return false }, "no longer exists"},
	}
	for _, planted := range cases {
		findings := ledgerDrift(planted.frozen, planted.counted, planted.exists)
		if len(findings) != 1 || !strings.Contains(findings[0], planted.want) {
			t.Errorf("%s: want one finding containing %q, got %q", planted.name, planted.want, findings)
		}
	}
	if findings := ledgerDrift(map[string]int{"a_test.go": 2}, map[string]int{"a_test.go": 2}, present); len(findings) != 0 {
		t.Errorf("a file at its frozen count: want no finding, got %q", findings)
	}
}

// ledgerDrift returns every disagreement between the frozen ledger and the
// counted tree. The ledger states each current count, so a count that fell is a
// finding too: an entry left above its file is headroom the file can grow back
// into unseen.
func ledgerDrift(frozen, counted map[string]int, exists func(string) bool) []string {
	var findings []string
	for path, count := range counted {
		was, admitted := frozen[path]
		switch {
		case !admitted:
			findings = append(findings, fmt.Sprintf("%s: %d wall-clock read(s) in a test file that had none. "+
				"Date the fixture from clocktest.Now(t) instead, so the drift lane can move it; if the "+
				"instant genuinely must be wall time, add the file to %s with its count and say why in "+
				"the commit.", path, count, ledgerPath))
		case count > was:
			findings = append(findings, fmt.Sprintf("%s: %d wall-clock read(s), up from the frozen %d. "+
				"The ledger only falls; new fixtures in an admitted file take their instant from "+
				"clocktest.Now(t).", path, count, was))
		case count < was:
			findings = append(findings, fmt.Sprintf("%s: %d wall-clock read(s), down from the frozen %d. "+
				"In %s, lower the frozen count to %d so the file cannot grow back.",
				path, count, was, ledgerPath, count))
		}
	}
	for path := range frozen {
		switch {
		case counted[path] > 0:
		case !exists(path):
			findings = append(findings, fmt.Sprintf("%s: %s names a file that no longer exists. A stale "+
				"entry hides the next file that takes its place.", ledgerPath, path))
		default:
			findings = append(findings, fmt.Sprintf("%s: %s now reads no wall clock. Remove its entry, "+
				"which is what holds it at zero.", ledgerPath, path))
		}
	}
	return findings
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
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
