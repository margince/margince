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
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	reachFixtures = "gates/testdata/individualperformancereach/"
	fixturePath   = "example.test/"
)

// fixtureConfig declares the planted auth package's reader and figure-free
// function, the planted transaction, runner and sentinels, and the tool
// registry a planted package declares.
func fixtureConfig(name string) walkConfig {
	config := walkConfig{
		readers:             []switchReader{{fixturePath + "auth", "IndividualPerformanceOf", 0}},
		bare:                []switchReader{{fixturePath + "auth", "Catalog", 0}},
		refusalConstructors: []string{"fmt.Errorf", "errors.New", fixturePath + "httperr.Validation"},
		sentinelPackages:    []string{fixturePath + "apperrors"},
	}
	switch name {
	case "toolroute":
		config.dispatchers = []switchReader{{fixturePath + "toolroute", "Registry.Invoke", 0}}
		config.synchronous = []string{fixturePath + "runner.Run"}
	case "everypath":
		config.synchronous = []string{fixturePath + "database.DB.Tx"}
	case "polarity":
		config.floors = []switchReader{{fixturePath + "polarity", "thinGroups", 0}}
	}
	return config
}

// loadFixture loads one planted package beside the planted auth reader. Each
// package is loaded alone, so a type one package converts is never wired
// into another.
func loadFixture(name string, config walkConfig) (*switchWalk, error) {
	fset := token.NewFileSet()
	byPackage := map[string][]*ast.File{}
	for _, pkg := range []string{"auth", name} {
		src, err := gatekit.ReadFile(reachFixtures + pkg + ".go.txt")
		if err != nil {
			return nil, err
		}
		file, err := parser.ParseFile(fset, pkg+".go", src, 0)
		if err != nil {
			return nil, err
		}
		byPackage[fixturePath+pkg] = []*ast.File{file}
	}
	g, err := newSwitchGraph(fset, byPackage, config)
	if err != nil {
		return nil, err
	}
	return newSwitchWalk(g), nil
}

// walkFixture answers which of ops the walk flags in the planted package name.
func walkFixture(t *testing.T, name string, ops ...string) []string {
	t.Helper()
	scoped := make([]scopedOperation, 0, len(ops))
	for _, id := range ops {
		scoped = append(scoped, scopedOperation{family: "fixture", method: "get", path: "/" + id, id: id})
	}
	return flaggedIn(t, name, scoped)
}

func flaggedIn(t *testing.T, name string, ops []scopedOperation) []string {
	t.Helper()
	walk, err := loadFixture(name, fixtureConfig(name))
	if err != nil {
		t.Fatal(err)
	}
	var flagged []string
	for _, op := range ops {
		census, paths := surfaceFindings(walk, op)
		if len(census) > 0 {
			t.Fatalf("the fixture %s has no handler for %s: %v", name, op.id, census)
		}
		if len(paths) > 0 {
			flagged = append(flagged, op.id)
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
		"getPastADerivedAnswer", "getThroughARespondingHelper", "getUnlessRefused",
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

// A reader's answer checks only on a branch a condition on it decides. A
// dropped, blank or unread answer checks nothing, and so does a dropped refusal.
func TestAnAnswerNoConditionReadsChecksNothing(t *testing.T) {
	t.Parallel()
	ops := []string{"getAfterADroppedAnswer", "getAfterABlankAnswer", "getAfterAnUnreadAnswer", "getAfterADroppedRefusal", "getUnlessBoundOff"}
	if got := walkFixture(t, "ignoredanswer", ops...); !slices.Equal(got, ops) {
		t.Errorf("flagged %v, want %v", got, ops)
	}
}

// A reader is known by its package's import path, so a package that shares
// the switch's package name lends no check.
func TestAReaderOfTheSameNameInAnotherPackageChecksNothing(t *testing.T) {
	t.Parallel()
	if got := walkFixture(t, "impostor", "getThroughAnImpostor"); !slices.Equal(got, []string{"getThroughAnImpostor"}) {
		t.Errorf("flagged %v, want getThroughAnImpostor", got)
	}
}

// A callback handed to sync.Once, to a registration or to an iterator may
// run once in the process, later, or never. Its check covers nothing.
func TestACallbackAnOutsideCalleeKeepsOrSkipsLendsNoCheck(t *testing.T) {
	t.Parallel()
	ops := []string{"getAfterOnce", "getAfterARegistration", "getAfterAnIterator"}
	if got := walkFixture(t, "heldcallback", ops...); !slices.Equal(got, ops) {
		t.Errorf("flagged %v, want %v", got, ops)
	}
}

// A type error no stand-in import explains fails the walk, which would read
// that code wrong. So does a synchronous callee no package calls.
func TestATypeErrorOrAnUncalledSynchronousCalleeFailsTheWalk(t *testing.T) {
	t.Parallel()
	_, err := loadFixture("typeerror", fixtureConfig("typeerror"))
	if err == nil || !strings.Contains(err.Error(), "typeerror.go") {
		t.Errorf("a mistyped figure gave %v, want an error naming the file", err)
	}
	config := fixtureConfig("everypath")
	config.synchronous = append(config.synchronous, fixturePath+"database.DB.Never")
	if _, err := loadFixture("everypath", config); err == nil || !strings.Contains(err.Error(), "DB.Never") {
		t.Errorf("an uncalled synchronous callee gave %v, want an error naming it", err)
	}
}

// An err name proves nothing. A nil package variable is no refusal, nor is
// one set from a call that may answer nil, nor an undeclared package's Err
// name. The success beside each is unchecked.
func TestAnErrorNamedLikeASentinelThatMayBeNilIsNoRefusal(t *testing.T) {
	t.Parallel()
	ops := []string{"getUnlessLast", "getUnlessLooked", "getUnlessGone"}
	if got := walkFixture(t, "notsentinel", ops...); !slices.Equal(got, ops) {
		t.Errorf("flagged %v, want %v", got, ops)
	}
}

// An operation added beside a family's paths, under a path no family names,
// is in the census and walked.
func TestAnOperationBesideTheFamilysPathsIsWalked(t *testing.T) {
	t.Parallel()
	src, err := gatekit.ReadFile(reachFixtures + "unplaced.yaml.txt")
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(src, &doc); err != nil {
		t.Fatal(err)
	}
	paths, _ := doc["paths"].(map[string]any)
	ops, err := scopedOperations(paths, []string{"reports"}, map[string][]string{"reports": {"/analytics/reports"}})
	if err != nil {
		t.Fatal(err)
	}
	if got := flaggedIn(t, "unplaced", ops); !slices.Equal(got, []string{"getBrandNew"}) {
		t.Errorf("flagged %v, want getBrandNew: it lies under /analytics beside the placed report", got)
	}
}

// A loop entered with no figure is covered when every pass ends checked, and
// a loop that runs no time adds nothing. A pass that skips the check, or a
// figure held from before the loop, still fails.
func TestALoopIsCoveredOnlyWhenEveryPassEndsChecked(t *testing.T) {
	t.Parallel()
	got := walkFixture(t, "loopcheck", "getEachRowChecked", "getEachRowSkippingOnePass", "getAfterAnUncheckedFigure", "getNoRows")
	if want := []string{"getEachRowSkippingOnePass", "getAfterAnUncheckedFigure"}; !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

// A comma-ok read of a map field checks on its hit branch when every write
// to that field stores a checked value. A field also filled unchecked, a read
// with no ok, and the miss branch check nothing.
func TestACacheChecksOnlyOnAHitOfAFieldFilledOnlyChecked(t *testing.T) {
	t.Parallel()
	got := walkFixture(t, "cache", "getThroughAHeldCache", "getThroughALeakyCache", "getThroughAPlainRead", "getOnTheMissBranch")
	if want := []string{"getThroughALeakyCache", "getThroughAPlainRead", "getOnTheMissBranch"}; !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

// A bool answer checks only where it permits, and a floor's only where it
// does not withhold. The team's figure on the off branch, or inside the
// floor, fails; a refusal there, or the floor passed first, does not.
func TestAnAnswerChecksOnlyOnTheBranchThatPermits(t *testing.T) {
	t.Parallel()
	ops := []string{"getTheTeamWhileOff", "getTheTeamUnlessOff", "getTheTeamOrTheFloor", "getPastTheFloor", "getInsideTheFloor"}
	if got, want := walkFixture(t, "polarity", ops...), []string{"getTheTeamWhileOff", "getInsideTheFloor"}; !slices.Equal(got, want) {
		t.Errorf("flagged %v, want %v", got, want)
	}
}

// A figure-free call cannot make a path figure-free once it holds an
// unchecked figure.
func TestAFigureFreeCallAfterAFigureCoversNothing(t *testing.T) {
	t.Parallel()
	if got := walkFixture(t, "polarity", "getAFigureThenTheCatalog"); !slices.Equal(got, []string{"getAFigureThenTheCatalog"}) {
		t.Errorf("flagged %v, want getAFigureThenTheCatalog", got)
	}
}
