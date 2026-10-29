// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every capability the extension tier publishes must have a live unit
// declaring it.
//
// A capability nothing declares is a capability nothing exercises: it compiles,
// it has unit tests around its own seam, and the composed path from a
// declaration through the generator, the boot reconciliation and the running
// process is walked by nobody. The tier used to answer this by accident — one
// demo unit declared everything, so a break showed up somewhere — and an
// accident is not a gate: the day that unit is retired the coverage leaves with
// it and no assertion notices.
//
// WHAT DECIDES THE LIST. The capabilities come from `extension.Extension`,
// whose package doc states the rule this reads: "Capabilities are fields; a new
// capability kind is a new field". A list written here instead would be a
// second copy of that struct, and the copy that goes short is the one that
// still passes. The layers come from the composer's own `*Layer` constants
// (tools/gen-composition), which are what decide the directories a unit ships.
//
// WHY THE DECLARATION AND NOT THE MANIFEST. manifest.generated.json is the
// composed publication and it is what says a unit is IN the tier, so it is the
// corpus here. It is not the evidence: it publishes what an operator must
// resolve, which is six of the capability fields — a unit's migrations,
// jurisdiction packs and failure classes never reach it — so a census reading
// only manifests would report a clean sweep over capabilities it cannot see.
// The declaration the manifest is generated FROM carries all of them.
//
// A staged fixture counts as a declarer. CI requires it in instance/go.mod and runs the
// composed build, the unit lane and its self-tests over it. The workflow's own
// staging step names it; a fixture nothing stages is walked by nobody.

import (
	"go/ast"
	"go/parser"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/pkg/extension"
)

const (
	// extensionSurfaceFile declares the tier's capability fields.
	extensionSurfaceFile = "pkg/extension/extension.go"
	// composerDir owns the layer constants — which subdirectory of a unit
	// holds its contract fragments, its SQL and its screen.
	composerDir = "tools/gen-composition"
	// tierDir is the enabled set. Presence under it IS the enablement, so
	// what it holds is what "live" means here.
	tierDir = "../extensions"
	// ciWorkflow is the workflow whose extension lane stages fixtures.
	ciWorkflow = "../.github/workflows/ci.yml"
)

// stagingStep matches a workflow step requiring one fixture unit from its folder.
var stagingStep = regexp.MustCompile(`=> \.\./fixtures/extensions/([\w-]+)`)

// identityFields are the two fields of extension.Extension that name the unit
// rather than granting it anything. They are excluded by NAME, and their
// absence is a failure rather than a smaller list: a rename that slipped past
// this would quietly add "Name" to the census and demand a unit declare it.
var identityFields = []string{"Name", "Version"}

// unit is one live unit: the directory under extensions/, what its declaration
// asks for, and which layers it ships.
type unit struct {
	name         string
	capabilities []string
	layers       []string
}

// TestEveryPublishedCapabilityHasALiveDeclaringUnit is the census. A capability
// field with no declarer names itself in the failure, because the answer is
// either to declare it in a unit or to delete the field.
func TestEveryPublishedCapabilityHasALiveDeclaringUnit(t *testing.T) {
	t.Parallel()
	units := append(liveUnits(t), stagedFixtures(t)...)
	for _, capability := range undeclared(publishedCapabilities(t), units) {
		t.Errorf("no live unit or staged fixture declares extension.Extension.%s — the composed path "+
			"from that declaration to the running process is walked by nothing, so it breaks silently. "+
			"Declare it in a unit under extensions/ or a fixture CI stages, or delete the field.", capability)
	}
}

// undeclared is the capabilities no unit among declarers sets.
func undeclared(capabilities []string, declarers []unit) []string {
	var missing []string
	for _, capability := range capabilities {
		if len(unitsWhere(declarers, func(u unit) bool { return slices.Contains(u.capabilities, capability) })) == 0 {
			missing = append(missing, capability)
		}
	}
	return missing
}

// A field only a staged fixture declares is walked; a field nobody declares is
// reported, whichever kind of unit is missing.
func TestTheCensusCountsAStagedFixtureAndStillReportsAFieldNobodyDeclares(t *testing.T) {
	t.Parallel()
	tier := unit{name: "tracked", capabilities: []string{"Tools"}}
	fixture := unit{name: "staged", capabilities: []string{"SystemReads"}}
	got := undeclared([]string{"Tools", "SystemReads", "Inbound"}, []unit{tier, fixture})
	if !slices.Equal(got, []string{"Inbound"}) {
		t.Errorf("undeclared = %v, want only the field nobody declares, Inbound", got)
	}
	if got := undeclared([]string{"SystemReads"}, []unit{tier}); !slices.Equal(got, []string{"SystemReads"}) {
		t.Errorf("without the fixture, undeclared = %v, want SystemReads reported", got)
	}
}

// TestEveryComposedLayerIsShippedByALiveUnit covers what the declaration cannot
// say: a unit's contract fragments, SQL and screen are DIRECTORIES the composer
// reads, and each has a lane of its own that nothing else keeps honest. The
// frontend one is why this arm exists at all — `make fe-test-ext` globs
// extensions/*/frontend for suites and passes over a tier that ships none.
func TestEveryComposedLayerIsShippedByALiveUnit(t *testing.T) {
	t.Parallel()
	units := liveUnits(t)
	for _, layer := range composedLayers(t) {
		shippers := unitsWhere(units, func(u unit) bool { return slices.Contains(u.layers, layer) })
		if len(shippers) == 0 {
			t.Errorf("no live unit ships a %s/ layer — every lane that reads it now reads an empty "+
				"tier, and an empty tier is what a lane cannot tell from a clean one", layer)
		}
	}
}

// TestTheCapabilityCensusReadsTheWholeTier asserts the CORPUS, before any
// verdict rests on it. Each of the three readings can fall short silently — a
// moved surface file yields no capabilities, a moved tier yields no units, a
// declaration this reader cannot follow yields an empty unit — and every one of
// them reports a clean sweep.
func TestTheCapabilityCensusReadsTheWholeTier(t *testing.T) {
	t.Parallel()

	capabilities := publishedCapabilities(t)
	if len(capabilities) == 0 {
		t.Errorf("%s yielded no capability field — a census with nothing to look for passes over "+
			"anything", extensionSurfaceFile)
	}
	if len(composedLayers(t)) == 0 {
		t.Errorf("%s declares no layer constant — the layer arm would be checking nothing", composerDir)
	}

	units := liveUnits(t)
	if len(units) == 0 {
		t.Fatalf("%s holds no live unit — with an empty tier every capability is undeclared and "+
			"this census would report the tier clean", tierDir)
	}
	fixtures := stagedFixtures(t)
	if len(fixtures) == 0 {
		t.Errorf("%s stages no fixture unit, or this census no longer reads its step — a field only "+
			"a fixture declares would then be reported, or one it stopped declaring missed", ciWorkflow)
	}
	for _, u := range append(units, fixtures...) {
		if len(u.capabilities) == 0 {
			t.Errorf("extensions/%s declares no capability at all — a unit exists to declare "+
				"something, so this is a reading that failed rather than a unit that asks for "+
				"nothing", u.name)
		}
	}
}

// publishedCapabilities is the field names of extension.Extension less the
// identity pair.
func publishedCapabilities(t *testing.T) []string {
	t.Helper()
	file := parseGo(t, extensionSurfaceFile)
	var fields []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.TypeSpec)
		if !ok || spec.Name.Name != "Extension" {
			return true
		}
		structType, ok := spec.Type.(*ast.StructType)
		if !ok {
			return false
		}
		for _, field := range structType.Fields.List {
			for _, name := range field.Names {
				fields = append(fields, name.Name)
			}
		}
		return false
	})
	for _, identity := range identityFields {
		if !slices.Contains(fields, identity) {
			t.Fatalf("extension.Extension has no %s field — this census excludes the identity pair by "+
				"name, so a rename here silently changes what it demands of every unit", identity)
		}
	}
	return slices.DeleteFunc(fields, func(f string) bool { return slices.Contains(identityFields, f) })
}

// layerConstant matches the composer's layer declarations: the constants saying
// which subdirectory of a unit holds each layer.
var layerConstant = regexp.MustCompile(`(?m)^const (\w+)Layer = (.+)$`)

// surfaceConstants resolves the one non-literal spelling a layer constant uses:
// the composer takes the migrations directory from the published surface rather
// than respelling it. Resolved through the constant ITSELF, so the two cannot
// disagree — only the identifier is named here.
//
// gatekit:fixture the published constant a layer declaration is spelled with
var surfaceConstants = map[string]string{"extension.MigrationsDir": extension.MigrationsDir}

// composedLayers is the unit subdirectories the composer reads.
func composedLayers(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(composerDir)
	if err != nil {
		t.Fatalf("reading %s: %v", composerDir, err)
	}
	var layers []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		for _, match := range layerConstant.FindAllStringSubmatch(readFile(t, filepath.Join(composerDir, e.Name())), -1) {
			layers = append(layers, resolveLayer(t, match[2]))
		}
	}
	sort.Strings(layers)
	return slices.Compact(layers)
}

// resolveLayer reads a layer constant's value. It fails on a spelling it cannot
// resolve rather than skipping it: a layer dropped from this list is a lane
// nobody is counting, which is what the arm above exists to refuse.
func resolveLayer(t *testing.T, expr string) string {
	t.Helper()
	if literal, err := strconv.Unquote(strings.TrimSpace(expr)); err == nil {
		return literal
	}
	if resolved, ok := surfaceConstants[strings.TrimSpace(expr)]; ok {
		return resolved
	}
	t.Fatalf("a layer constant in %s is spelled %s, which this census cannot resolve to a directory "+
		"name — resolve it through the constant itself in surfaceConstants", composerDir, expr)
	return ""
}

// liveUnits reads every unit in the enabled set. A unit is a directory holding
// a Go module; the generated manifest beside it is what says the composer
// reached it, and a module without one is a unit this census would otherwise
// drop without a word.
func liveUnits(t *testing.T) []unit {
	t.Helper()
	entries, err := os.ReadDir(tierDir)
	if err != nil {
		t.Fatalf("reading %s: %v", tierDir, err)
	}
	var units []unit
	for _, e := range entries {
		dir := filepath.Join(tierDir, e.Name())
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			continue // not a Go unit
		}
		if u, ok := readUnit(t, dir, e.Name()); ok {
			units = append(units, u)
		}
	}
	return units
}

// readUnit reads one unit directory. The generated manifest beside it is what
// says the composer reached it, and a unit without one is reported rather than
// dropped.
func readUnit(t *testing.T, dir, name string) (unit, bool) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "manifest.generated.json")); err != nil {
		t.Errorf("%s is a Go unit with no manifest.generated.json — the composer has not reached "+
			"it, so nothing here can say what it declares (run `make gen`)", dir)
		return unit{}, false
	}
	return unit{name: name, capabilities: declaredCapabilities(t, dir), layers: shippedLayers(t, dir)}, true
}

// stagedFixtures reads every fixture unit the CI workflow requires.
// A staged name with no unit behind it fails: the step would require nothing.
func stagedFixtures(t *testing.T) []unit {
	t.Helper()
	var units []unit
	for _, step := range stagingStep.FindAllStringSubmatch(readFile(t, ciWorkflow), -1) {
		dir := filepath.Join("..", "fixtures", "extensions", step[1])
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err != nil {
			t.Errorf("%s stages fixtures/extensions/%s, which is not a Go unit", ciWorkflow, step[1])
			continue
		}
		if u, ok := readUnit(t, dir, step[1]); ok {
			units = append(units, u)
		}
	}
	return units
}

// declaredCapabilities is the fields a unit's declaration sets, less the
// identity pair: a unit that sets only Name and Version has granted itself
// nothing, and the census that follows depends on that unit reading as empty.
func declaredCapabilities(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading %s: %v", dir, err)
	}
	var declared []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		ast.Inspect(parseGo(t, filepath.Join(dir, e.Name())), func(n ast.Node) bool {
			literal, ok := n.(*ast.CompositeLit)
			if !ok || !isExtensionDeclaration(literal.Type) {
				return true
			}
			for _, element := range literal.Elts {
				pair, ok := element.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				if key, ok := pair.Key.(*ast.Ident); ok {
					declared = append(declared, key.Name)
				}
			}
			return true
		})
	}
	sort.Strings(declared)
	return slices.DeleteFunc(slices.Compact(declared), func(f string) bool { return slices.Contains(identityFields, f) })
}

// isExtensionDeclaration reports whether a composite literal is an
// extension.Extension value.
func isExtensionDeclaration(expr ast.Expr) bool {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Extension" {
		return false
	}
	pkg, ok := selector.X.(*ast.Ident)
	return ok && pkg.Name == "extension"
}

// shippedLayers is the layer directories a unit actually ships. It walks
// rather than stat-ing the unit root: a layer may sit inside another — a
// unit's copy lives in its frontend package — and the composer names each one
// by its own directory, not by a path from the unit root.
//
// node_modules is skipped because a dependency's own directories are not this
// unit's layers, and it is where the walk would otherwise spend its time.
func shippedLayers(t *testing.T, dir string) []string {
	t.Helper()
	layers := composedLayers(t)
	var shipped []string
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !entry.IsDir() {
			return nil
		}
		if entry.Name() == "node_modules" {
			return fs.SkipDir
		}
		if path != dir && slices.Contains(layers, entry.Name()) {
			shipped = append(shipped, entry.Name())
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", dir, err)
	}
	sort.Strings(shipped)
	return slices.Compact(shipped)
}

// unitsWhere is the units satisfying a predicate.
func unitsWhere(units []unit, match func(unit) bool) []unit {
	var found []unit
	for _, u := range units {
		if match(u) {
			found = append(found, u)
		}
	}
	return found
}

// parseGo parses one file. Bodies included: a unit's declaration is built
// inside New(), which is where the composer's own reader finds it too.
func parseGo(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	return file
}
