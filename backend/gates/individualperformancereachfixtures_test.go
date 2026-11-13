// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind falsification H2

//go:build !integration

package gates

// The individual-performance path walk read against planted packages, one
// shape per package, each of which an any-path reading of the switch accepts.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const reachFixtures = "gates/testdata/individualperformancereach/"

// fixtureDispatchers names the tool registry a planted package declares.
var fixtureDispatchers = map[string][]switchReader{"toolroute": {{"toolroute", "Registry.Invoke"}}}

// walkFixture loads one planted package beside the planted auth reader, and
// answers which of ops the walk flags. Each package is loaded alone, so a type
// one package converts is never wired into another.
func walkFixture(t *testing.T, name string, ops ...string) []string {
	t.Helper()
	fset := token.NewFileSet()
	byPackage := map[string][]*ast.File{}
	for _, pkg := range []string{"auth", name} {
		src, err := gatekit.ReadFile(reachFixtures + pkg + ".go.txt")
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(fset, pkg+".go", src, 0)
		if err != nil {
			t.Fatal(err)
		}
		byPackage["example.test/"+pkg] = []*ast.File{file}
	}
	g := newSwitchGraph(fset, byPackage)
	if err := g.readers([]switchReader{{"auth", "IndividualPerformanceOf"}}, []switchReader{{"auth", "Catalog"}}); err != nil {
		t.Fatal(err)
	}
	if err := g.dispatchers(fixtureDispatchers[name]); err != nil {
		t.Fatal(err)
	}
	walk := newSwitchWalk(g)
	var flagged []string
	for _, id := range ops {
		census, paths := surfaceFindings(walk, scopedOperation{family: "fixture", method: "get", path: "/" + id, id: id})
		if len(census) > 0 {
			t.Fatalf("the fixture %s has no handler for %s: %v", name, id, census)
		}
		if len(paths) > 0 {
			flagged = append(flagged, id)
		}
	}
	return flagged
}

func TestAnUncheckedImplementationThatIsWiredFailsBesideACheckedOneThatIsNot(t *testing.T) {
	t.Parallel()
	if got := walkFixture(t, "wiredimpl", "getFigures"); !slices.Equal(got, []string{"getFigures"}) {
		t.Errorf("flagged %v, want getFigures: only the unchecked implementation is wired", got)
	}
}

func TestACallbackResolvesPerCallSiteAndAFieldByEveryValueItIsGiven(t *testing.T) {
	t.Parallel()
	got := walkFixture(t, "callbacktwice", "getChecked", "getUnchecked", "getThroughField")
	if want := []string{"getUnchecked", "getThroughField"}; !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

func TestABranchThatSkipsTheCheckBesideOneThatAsksFails(t *testing.T) {
	t.Parallel()
	ops := []string{
		"getProgress", "getProgressBySwitch", "getEachRowSkippingOne", "getEachRowCheckingInside", "getAfterTheCatalog", "getAFigureBesideARefusal",
		"getAFigureBesideASentinel", "getAFigureBesideAWrappedSentinel",
		"getAFigureBesideAnImportedSentinel", "getAFigureBesideAnErrorLiteral",
	}
	if got := walkFixture(t, "ownerbranch", ops...); !slices.Equal(got, ops) {
		t.Errorf("flagged %v, want %v", got, ops)
	}
}

func TestAHandlerThatChecksOnEveryPathPasses(t *testing.T) {
	t.Parallel()
	ops := []string{
		"getInATransaction", "getThroughTheWiredImplementation", "getThroughACheckedCallback", "getOnBothBranches",
		"getEachRow", "getThroughAConstructor", "getThroughABuiltReader", "getUnlessUnknown", "getUnlessMalformed", "getTheCatalog",
	}
	if got := walkFixture(t, "everypath", ops...); len(got) > 0 {
		t.Errorf("flagged %v, want none: each checks the switch before every figure it writes", got)
	}
}

// A handler whose response the walk cannot see as a figure fails, rather than
// passing with nothing checked.
func TestAHandlerWithNoFigureTheWalkSeesFailsTheCensus(t *testing.T) {
	t.Parallel()
	if got := walkFixture(t, "nofigure", "getNothingItCanSee"); !slices.Equal(got, []string{"getNothingItCanSee"}) {
		t.Errorf("flagged %v, want getNothingItCanSee", got)
	}
}

// A route that hands a tool's name to the registry answers for that tool alone.
func TestARouteTheRegistryServesAnswersForTheToolItNames(t *testing.T) {
	t.Parallel()
	ops := []string{"getCheckedTool", "getUncheckedTool", "getToolByPath", "getRenamedTool", "getRewiredTool", "getToolThroughARunner"}
	if got, want := walkFixture(t, "toolroute", ops...), ops[1:]; !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}
