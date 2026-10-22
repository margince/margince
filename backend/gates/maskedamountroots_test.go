// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// What the deal-amount census's roots are worth.
//
// TestEveryReaderOfADealAmountCarriesTheMaskOrAVerdict sweeps `internal` and
// nothing else, which is a claim as much as a configuration: every statement
// that can read a deal's money lives there. A statement outside it would not
// fail that census — it would be invisible to it, and the census would report
// PASS over the one read nobody masked. That is the failure §"Rules learned
// from the review loop" item 8 names: a census that reads a smaller tree,
// reports PASS, and leaves no assertion to notice.
//
// It also carries the other half of the same premise, which is why it exists
// as one gate rather than two. The census reads STATEMENTS, so a package that
// takes the figure as a Go VALUE — companybrief folding `deal.Amount` out of a
// Company360, the slipping tool folding it out of a listed Deal — is invisible
// to it by construction. That class is nonetheless covered, and covered by
// composition rather than by luck: a contracts money field is only ever
// produced by SQL, every such statement is in `internal`, and the census
// subjects all of those. The composition holds exactly while this does.
//
// So: nothing outside `internal` may hold a read the census would subject. The
// binaries compose, and the published surface is stdlib-only by its own gate.
// An extension unit reaches core data through the seam. None of the three has
// business writing this statement, and the census cannot see one that does.
//
// The census's own subject, not a wider one. A unit asking Authority().CanRead
// names `amount_minor` as a field in a Go list and reads no table.

import (
	"go/parser"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// outsideInternal is the register of files outside internal/ that hold such a
// read, each with why the census need not see it. Empty, and the emptiness is
// the finding.
var outsideInternal = gatekit.Waive(map[string]string{})

func TestNoDealAmountStatementLivesWhereTheCensusCannotSeeIt(t *testing.T) {
	t.Parallel()

	// The same subject the census sweeps with, asked of the same derivation
	// rather than spelled again — a second spelling here is how this gate would
	// come to guard a narrower column set than the census it exists to bound.
	columns := dealMaskedColumns(t)
	builders := dealAmountBuilderNames(t, columns)
	roots := moduleRootsBesideInternal(t)
	// A walk that found no root is a walk that agrees with everything.
	if len(roots) == 0 {
		t.Fatal("this gate found no tree beside internal/ to read, so it would report a clean " +
			"result over any statement placed in one")
	}

	var offenders []string
	for _, root := range roots {
		err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil || info.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			// The SAME membership the census applies (gatekit.isSweptSource):
			// generated output, a test, and anything under testdata are not
			// behaviour a gate judges. Matching it is what makes this gate a
			// statement about the census's reach rather than a second and
			// wider rule — three migration suites assert the column exists,
			// and a schema assertion serves no caller a figure.
			if !sweptLikeTheCensus(filepath.ToSlash(path)) {
				return nil
			}
			file, parseErr := gatekit.ParseFile(path, 0)
			if parseErr != nil {
				return parseErr
			}
			if readsADealAmount(path, file, columns, builders) && !outsideInternal.Waived(t, filepath.ToSlash(path)) {
				offenders = append(offenders, filepath.ToSlash(path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	// An entry that matched nothing would read as ratification of a file that
	// is gone, which is the register saying something false rather than
	// nothing.
	outsideInternal.AssertAllMatched(t)

	if len(offenders) > 0 {
		t.Errorf("%d file(s) outside internal/ read a deal amount, where the masked-amount "+
			"census cannot see them:\n\t%s\n\n"+
			"That census sweeps `internal` alone, so a statement here is not a failure it reports — "+
			"it is one it never reads, and it would go on answering PASS. Move the statement into a "+
			"module, or widen maskedAmountScope's Roots and say here why the tree it now reads is "+
			"the whole of it.",
			len(offenders), strings.Join(offenders, "\n\t"))
	}
}

// TestTheRootsGateJudgesReadsAndNotFieldNames holds the gate's boundary. A unit
// naming the money field to the seam reads nothing. A statement reading the
// deal's money is what the gate exists for.
func TestTheRootsGateJudgesReadsAndNotFieldNames(t *testing.T) {
	t.Parallel()
	columns := dealMaskedColumns(t)
	builders := dealAmountBuilderNames(t, columns)
	for _, c := range []struct {
		name   string
		source string
		reads  bool
	}{
		{
			name:   "a unit's field list for the authority seam",
			source: "var cardFields = []string{\"name\", \"stage_id\", \"amount_minor\", \"currency\"}",
		},
		{
			name:   "a statement reading the deal's money",
			source: "func total() string { return `SELECT sum(d.amount_minor) FROM deal d WHERE d.archived_at IS NULL` }",
			reads:  true,
		},
		{
			name:   "the converted column read the same way",
			source: "func base() string { return \"SELECT d.amount_minor_base \" + \"FROM deal d\" }",
			reads:  true,
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			file, err := parser.ParseFile(gatekit.SourceFileSet(), "unit.go", "package unit\n"+c.source, 0)
			if err != nil {
				t.Fatalf("parsing the fixture: %v", err)
			}
			if got := readsADealAmount("unit.go", file, columns, builders); got != c.reads {
				t.Fatalf("readsADealAmount = %v, want %v", got, c.reads)
			}
		})
	}
}

// sweptLikeTheCensus is gatekit's own membership rule, asked of a path.
//
// Spelled here because the census reaches it through gatekit.Scope, which
// walks roots rather than answering about one file. The two must agree: a
// file this admitted and the census did not would be reported as out of the
// census's reach when it was never in anybody's.
func sweptLikeTheCensus(path string) bool {
	if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
		return false
	}
	return !slices.Contains(strings.Split(path, "/"), "testdata")
}

// moduleRootsBesideInternal gathers the backend module's top-level directories
// apart from the one the census reads. It adds the sibling Go trees the craft
// gate treats as product code.
func moduleRootsBesideInternal(t *testing.T) []string {
	t.Helper()
	var roots []string
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the backend module: %v", err)
	}
	for _, entry := range entries {
		// `internal` is the census's own tree; `gates` is this file's.
		if !entry.IsDir() || entry.Name() == "internal" || entry.Name() == "gates" ||
			strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		roots = append(roots, entry.Name())
	}
	// The trees outside the module that ship Go the same way, named relative to
	// it. An extension unit reaches core data through the published seam, so a
	// deal-amount statement in one is the shape this gate most wants to catch.
	for _, beside := range []string{"../extensions", "../fixtures", "../desktop"} {
		if _, err := os.Stat(beside); err == nil {
			roots = append(roots, beside)
		}
	}
	return roots
}
