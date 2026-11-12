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
const (
	composeRootProductionFiles = 1366
	composeRootTestFiles       = 1885
	composeRootProductionLines = 228_055
	composeRootTestLines       = 354_832
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
