// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind reachability H2

package gates

// Only the erased subject lifts an erasure, and only by answering the link
// mailed to the address. The one statement that lifts a row sits in
// privacy.LiftEmailSuppressionTx. Its one caller is the confirm step of the
// booking page's return door. That door is a human-only operation, so no agent
// tool reaches it. The corpus is every hand-written Go file. A second lifting
// statement fails here, and so does a second caller of the lift.

import (
	"cmp"
	"go/ast"
	"go/parser"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// liftsErasure matches SQL that ends an entry's refusal: setting lifted_at to
// a value, or removing rows from the list.
var (
	setsLift        = regexp.MustCompile(`(?i)\blifted_at\s*=\s*([a-z_@$(][^,\s]*)`)
	deletesFromList = regexp.MustCompile(`(?i)\bDELETE\s+FROM\s+erasure_suppression\b`)
)

// The chain from the lift to the route, one call at a time.
const (
	liftFunction         = "LiftEmailSuppressionTx"
	liftSeat             = "internal/modules/privacy/suppressionlift.go:" + liftFunction
	returnStore          = "storeReturned"
	returnStoreSeat      = "internal/compose/erasurereturn.go:" + returnStore
	returnConfirmHandler = "ConfirmBookingReturn"
	returnConfirmSeat    = "internal/compose/erasurereturn.go:" + returnConfirmHandler
	returnConfirmOp      = "confirmBookingReturn"
)

// erasureListMovers write lifted_at or remove rows without lifting an armed
// entry, each with why.
var erasureListMovers = gatekit.Waive(map[string]string{
	"internal/modules/privacy/identifierrekey.go:RekeySuppressionListPageTx": "moves a row to its keyed hash with the row's own lift, and an armed twin keeps the keyed row armed",
})

func TestOnlyTheMailedLinkLiftsAnErasure(t *testing.T) {
	t.Parallel()
	defer erasureListMovers.AssertAllMatched(t)
	files := productionGoFiles(t)

	var lifters []string
	for _, file := range files {
		for _, decl := range file.File.Decls {
			for _, read := range gatekit.DeclReads(decl, erasureListStatement) {
				if !liftsAnEntry(read.SQL) {
					continue
				}
				site := file.Path + ":" + read.Function
				if site != liftSeat && !erasureListMovers.Waived(t, site) {
					t.Errorf("%s lifts an erasure (%s). Only %s may: the erased subject lifts their "+
						"own erasure by answering the mailed link, and no other path may", site, gatekit.FirstLineOf(read.SQL), liftSeat)
				}
				lifters = append(lifters, site)
			}
		}
	}
	if !slices.Contains(lifters, liftSeat) {
		t.Fatalf("%s holds no lifting statement; the census no longer sees the lift", liftSeat)
	}

	assertOnlyReferrer(t, files, liftFunction, returnStoreSeat)
	assertOnlyReferrer(t, files, returnStore, returnConfirmSeat)
	// The confirm handler is reached through the generated router alone.
	assertOnlyReferrer(t, files, returnConfirmHandler, "")
	if _, humanOnly := humanOnlyOperationsInContract(t)[returnConfirmOp]; !humanOnly {
		t.Errorf("%s is not x-agent-access: human-only in api/crm.yaml, so an agent tool could lift an erasure", returnConfirmOp)
	}
}

func TestTheLiftCensusSeesEachWayToLift(t *testing.T) {
	t.Parallel()
	for _, sql := range []string{
		"UPDATE erasure_suppression SET lifted_at = now()",
		"UPDATE erasure_suppression SET lift_consent_event_id = $2, lifted_at = $1",
		"UPDATE erasure_suppression SET lifted_at = @at",
		"DELETE FROM erasure_suppression WHERE kind = 'email'",
	} {
		if !liftsAnEntry(sql) {
			t.Errorf("the census would not see %q lift an erasure", sql)
		}
	}
	if liftsAnEntry("INSERT INTO erasure_suppression (kind) VALUES ($1) ON CONFLICT (kind) DO UPDATE SET lifted_at = NULL") {
		t.Error("the census counts clearing a lift as a lift")
	}
}

func liftsAnEntry(sql string) bool {
	if deletesFromList.MatchString(sql) {
		return true
	}
	for _, set := range setsLift.FindAllStringSubmatch(sql, -1) {
		if !strings.EqualFold(set[1], "NULL") {
			return true
		}
	}
	return false
}

// assertOnlyReferrer fails unless every reference to name in the tree sits in
// want: a call, a function value or a method value alike. An empty want
// admits none. The declaration's own name is not a reference.
func assertOnlyReferrer(t *testing.T, files []gatekit.ParsedFile, name, want string) {
	t.Helper()
	referrers := nameReferrers(files, name)
	if want != "" && len(referrers) == 0 {
		t.Errorf("nothing refers to %s; the census no longer sees the chain from the route to the lift", name)
	}
	for _, referrer := range referrers {
		if referrer != want {
			t.Errorf("%s refers to %s. Only %s may, so the mailed link stays the one way to lift an erasure",
				referrer, name, cmp.Or(want, "the generated router"))
		}
	}
}

// nameReferrers answers each declaration that refers to name, by file and
// declaration. A package-level reference is filed under "package scope".
func nameReferrers(files []gatekit.ParsedFile, name string) []string {
	var referrers []string
	for _, file := range files {
		for _, decl := range file.File.Decls {
			site, declared := file.Path+":package scope", (*ast.Ident)(nil)
			if fn, ok := decl.(*ast.FuncDecl); ok {
				site, declared = file.Path+":"+fn.Name.Name, fn.Name
			}
			ast.Inspect(decl, func(n ast.Node) bool {
				if ident, ok := n.(*ast.Ident); ok && ident != declared && ident.Name == name {
					referrers = append(referrers, site)
				}
				return true
			})
		}
	}
	return referrers
}

// productionGoFiles parses every hand-written, non-test Go file outside gates/.
func productionGoFiles(t *testing.T) []gatekit.ParsedFile {
	t.Helper()
	var files []gatekit.ParsedFile
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		if strings.HasSuffix(where, "_test.go") || strings.HasPrefix(where, "gates/") || isIntegrationTagged(path) {
			continue
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		files = append(files, gatekit.ParsedFile{Path: where, File: file})
	}
	return files
}
