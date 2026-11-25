// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H3

//go:build !integration

package gates

// The root compose package may not grow.
//
// It is one Go package that imports every module, so every lint, vet and test
// run analyses all of it after a change anywhere below. Its test files are most
// of that cost, so they are held as well. New feature code goes in a subpackage
// under internal/compose. A change that moves code out lowers these ceilings.

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const composeRootDir = "internal/compose"

// The ceilings sit a little above the size the package was when this gate
// landed, so branches already open could still merge. Lower one whenever a
// change moves code out. Raising one needs a reason in the pull request.
//
// The ceilings below hold develop and main together: each side's additions
// stayed under its own ceilings, and the two sets of additions sum to these.
const (
	composeRootProductionFiles = 1384
	// Raised by 2 files for the extension mail-file read cases and the purge
	// receipt cases, which drive this package's unexported seams. Raised by 1
	// production and 1 test file for the series-split fold and its cases.
	// Raised by 1 for the removed-unit list cases, which drive the unexported
	// Live List check and list export over a kind no unit declares. Raised by 1
	// test file for the data reset cases of an event the sweep orphaned.
	composeRootTestFiles = 1913
	// Raised by 107 for the floor of a figure every reader is shown alike. Raised
	// by 1 for the import that binds one zone lookup to each attention read.
	// Raised by 249 production and 62 test lines for the full-path certification
	// cases of two capture sites, which drive private engine code that cannot
	// leave this package. Raised by 58 test lines for the system health footer
	// cases, which read this package's unexported health sources. Raised by 29
	// test lines for the data reset cases of a timed-out job drain. Raised by 72
	// test lines for the extension mail-file and purge receipt cases. Raised by 30
	// production and 21 test lines for the approvals adapter and server option that
	// confirm a call in the conversation. Raised by 191 test lines for the
	// team-request mail cases, which drive this package's unexported mail worker
	// and send basis. Raised by 2 production and 52 test lines for the booking length
	// card cases, which drive the unexported bookingCard and the booking page fixtures.
	// Raised by 140 production and 269 test lines for the series-split fold and
	// repair task. Their cases drive the unexported Home meeting lane and fold.
	// Raised by 43 test lines for the refold cases of a transaction still open,
	// which drive the unexported clock refold worker.
	// Raised by 7 production lines for the team request reword and the paused
	// team link's title on Home. Raised by 64 test lines for the team-link mail
	// case, which drives the unexported mail worker over two team links.
	// Raised by 7 production and 106 test lines for the route answer that keeps
	// its field errors, and the route cases that read the unexported verb shapes.
	// Raised by 9 production and 17 test lines for the company status rename,
	// whose webhook and preview keys sit in this package's unexported maps.
	// Raised by 30 production and 363 test lines for lists over a removed
	// extension kind and the failing-report directory. The cases drive the
	// unexported Live List check, the list export and the report directory.
	// Raised by 12 production and 73 test lines for the reading refusal that
	// names its cause per question. Its cases drive the unexported document
	// extractor and transcript proposer.
	// Raised by 244 test lines for the data reset cases of an event the sweep
	// orphaned, which drive the unexported reset handlers.
	// Raised by 68 production and 96 test lines for path ids in extension routes
	// and the mail hooks in the extension inventory. They extend the unexported
	// route argument reader and inventory builder, and their cases drive them.
	// Raised by 6 production lines for the signals and initiatives carriers
	// every contact merge door is wired with. Raised by 6 test lines for the
	// seat without an introduction grant that the preview refusal case needs.
	composeRootProductionLines = 230_819
	composeRootTestLines       = 361_033
)

// A read under these floors stopped early or read the wrong place, and would
// pass every ceiling. They sit near 90% of the package; lower them with the
// ceilings when code moves out.
const (
	composeRootFloorFiles = 2900
	composeRootFloorLines = 520_000
)

// composeRootGenerated grows with the contract, not with hand-written code, so
// it is left out of the count. The gate fails when this list and the tree differ.
var composeRootGenerated = []string{
	"agentpolicy_gen.go",
	"jobkinds_gen.go",
	"queryenums_gen.go",
	"stubs_gen.go",
}

type composeRootSize struct {
	productionFiles, testFiles int
	productionLines, testLines int
	generated                  []string
}

func TestTheRootComposePackageDoesNotGrow(t *testing.T) {
	t.Parallel()
	size := measureComposeRoot(t, composeRootDir)
	if size.short() {
		t.Fatalf("read %d files and %d lines in %s, under the floors of %d and %d: "+
			"the census has gone short and would pass vacuously",
			size.productionFiles+size.testFiles, size.productionLines+size.testLines, composeRootDir,
			composeRootFloorFiles, composeRootFloorLines)
	}
	if !slices.Equal(size.generated, composeRootGenerated) {
		t.Errorf("%s holds the generated files %v, and this gate leaves out %v. Update composeRootGenerated",
			composeRootDir, size.generated, composeRootGenerated)
	}
	for _, measure := range []struct {
		what         string
		got, ceiling int
	}{
		{"production files", size.productionFiles, composeRootProductionFiles},
		{"test files", size.testFiles, composeRootTestFiles},
		{"production lines", size.productionLines, composeRootProductionLines},
		{"test lines", size.testLines, composeRootTestLines},
	} {
		if measure.got > measure.ceiling {
			t.Errorf("the root compose package has %d %s, over its ceiling of %d. Put new compose code in a "+
				"subpackage under internal/compose (docs/explanation/architecture.md), or move code out to make room",
				measure.got, measure.what, measure.ceiling)
		}
	}
}

// short reports a read too small to be the whole package.
func (size composeRootSize) short() bool {
	return size.productionFiles+size.testFiles < composeRootFloorFiles ||
		size.productionLines+size.testLines < composeRootFloorLines
}

// measureComposeRoot counts the Go files directly in dir that the Go tool
// builds. It leaves out subpackages, names starting with "." or "_", anything
// that is not a regular file, and the generated files it names apart.
func measureComposeRoot(t *testing.T, dir string) composeRootSize {
	t.Helper()
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("list %s: %v", dir, err)
	}
	var size composeRootSize
	for _, entry := range entries {
		name := entry.Name()
		if !entry.Type().IsRegular() || !strings.HasSuffix(name, ".go") ||
			strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		if strings.HasSuffix(name, "_gen.go") {
			size.generated = append(size.generated, name)
			continue
		}
		body, err := gatekit.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		lines := bytes.Count(body, []byte("\n"))
		if strings.HasSuffix(name, "_test.go") {
			size.testFiles++
			size.testLines += lines
			continue
		}
		size.productionFiles++
		size.productionLines += lines
	}
	slices.Sort(size.generated)
	return size
}

// The count is what the ceilings mean: root files only, tests apart, and the
// generated files named rather than counted.
func TestTheComposeRootCountReadsOnlyTheRootPackage(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	write := func(name, body string) {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	write("wiring.go", "package compose\n\nfunc a() {}\n")
	write("wiring_test.go", "package compose\n")
	write("lane_integration_test.go", "//go:build integration\n\npackage compose\n")
	write("stubs_gen.go", "package compose\n\nvar x = 1\n")
	write("notes.txt", "not go\n")
	write("feature/inner.go", "package feature\n\nfunc b() {}\n")
	write(".hidden.go", "package compose\n")
	write("_ignored.go", "package compose\n")
	if err := os.Symlink(filepath.Join(dir, "wiring.go"), filepath.Join(dir, "linked.go")); err != nil {
		t.Fatal(err)
	}
	got := measureComposeRoot(t, dir)
	want := composeRootSize{productionFiles: 1, testFiles: 2, productionLines: 3, testLines: 4, generated: []string{"stubs_gen.go"}}
	if got.productionFiles != want.productionFiles || got.testFiles != want.testFiles ||
		got.productionLines != want.productionLines || got.testLines != want.testLines ||
		!slices.Equal(got.generated, want.generated) {
		t.Fatalf("measured %+v, want %+v", got, want)
	}
}

// A read that stopped partway is caught by the floors, not passed as a small
// package. A read the size of the package is not.
func TestAComposeRootReadThatStopsEarlyIsShort(t *testing.T) {
	t.Parallel()
	stopped := composeRootSize{productionFiles: 500, testFiles: 500, productionLines: 90_000, testLines: 90_000}
	if !stopped.short() {
		t.Error("a read of the first 1,000 files passed the floors")
	}
	fewLines := composeRootSize{productionFiles: 1356, testFiles: 1875, productionLines: 100_000, testLines: 100_000}
	if !fewLines.short() {
		t.Error("a read of every file but a fraction of its lines passed the floors")
	}
	whole := composeRootSize{productionFiles: 1356, testFiles: 1875, productionLines: 227_127, testLines: 353_700}
	if whole.short() {
		t.Error("a read of the whole package was called short")
	}
}
