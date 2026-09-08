// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// A view's fixture is the tool's answer, and this is what makes that true.
//
// Every MCP App view renders `structuredContent.data` from one tool's result,
// and the member names its TypeScript reads are a hand-written mirror of the Go
// result struct. Nothing connected the two: a rename on the Go side compiles,
// ships, and produces a view that renders its EMPTY STATE — the views narrow
// every field through guards and answer empty rather than throwing, so the
// failure is contained and silent, and reads as "no data" rather than "the
// contract moved".
//
// And every test stays green, because the fixture each view is tested against
// is hand-built from the same reading. That is review-loop rule 6 exactly: a
// suite can be entirely green about a payload production no longer produces.
//
// So the fixture is the thing to hold. It is what the view's suite draws, so a
// fixture true to the contract makes those tests true to it too — and it is
// checkable from here, where the tool's own OUTPUT SCHEMA is available. The
// schema is derived from the Go type by reflection (agents/outputshapes.go), so
// a renamed struct member is a renamed schema property, and this gate goes red
// on the change that used to blank a panel.
//
// Nothing here is listed. The tools that carry a view declare it
// (mcp.ToolUI.ResourceURI), the view's directory is derived from that URI the
// same way the document's own file name is (agents/apps/fetch.go), and the
// members come from the schema and the fixture. A sixth view inherits the check
// by existing.

import (
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// appFixtureDir is where a view's hand-written fixture lives, keyed by the
// directory its URI derives to.
const appFixtureDir = "../frontend/src/mcp-apps"

// envelopeMembers are the ones the ENVELOPE owns rather than the tool. Every
// fixture is an Envelope wrapping the tool's data, so these two names appear in
// all of them and belong to no result struct.
var envelopeMembers = map[string]bool{envelopeDataMember: true, "warnings": true}

// envelopeDataMember is where a tool's own shape sits inside the envelope
// schema. It matches the constant the deriver writes; spelled here rather than
// imported because agents keeps it unexported, and a gate that reached for it
// would be asserting the deriver against itself.
const envelopeDataMember = "data"

// fixtureMemberOnlyInTheView ratifies a fixture member the schema does not
// publish. It is empty and meant to stay so: a member the view reads and the
// tool does not answer is the defect this gate is about, wearing the other
// direction.
var fixtureMemberOnlyInTheView = gatekit.Waive(map[string]string{})

// appFixtureFloor is the number of views the sweep must find. Below it the
// derivation has stopped reaching the catalog, and a sweep that judges nothing
// reports PASS.
const appFixtureFloor = 4

// fixtureKey matches one member of a JavaScript object literal. This tree's
// formatter puts one key per line, which is what makes a line-wise read of a
// fixture complete rather than approximate — and appFixtureFloor plus the
// per-fixture count below are what fail if that ever stops being true.
var fixtureKey = regexp.MustCompile(`^\s*([a-z][A-Za-z0-9_]*)\s*:`)

// fixtureString matches a quoted value, blanked before braces are counted so a
// `{` inside a message is not read as structure.
var fixtureString = regexp.MustCompile(`"[^"]*"`)

// dataMemberDepth is the brace depth of the members of the envelope's `data`.
// The literal opens at depth 0, `data:` sits at 1, and its own members at 2.
const dataMemberDepth = 2

func TestEveryAppViewFixtureMatchesItsToolsOutputSchema(t *testing.T) {
	t.Parallel()
	defer fixtureMemberOnlyInTheView.AssertAllMatched(t)

	checked := 0
	for _, spec := range compose.NewRegistry(nil, compose.SendPath{}).Specs() {
		if spec.UI == nil || spec.OutputSchema == nil {
			continue
		}
		dir := viewDirOf(spec.UI.ResourceURI)
		path := appFixtureDir + "/" + dir + "/fixture.ts"
		source, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// A view with no fixture is not a finding here: the geo probe
			// answers no tool's result and renders no record. What would be a
			// finding is a fixture that disagrees with a schema, and there is
			// none to disagree.
			continue
		}
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		checked++
		published, required := toolShapeMembers(t, spec.Name, spec.OutputSchema)
		if len(published) == 0 {
			t.Errorf("%s publishes an output schema with no properties, so this fixture is checked "+
				"against nothing", spec.Name)
			continue
		}
		fixture := fixtureMembers(source)
		if len(fixture) < 2 {
			t.Errorf("%s read %d member(s) out of %s — the fixture is not being read, and a fixture "+
				"nobody reads passes every comparison below", spec.Name, len(fixture), path)
			continue
		}
		compareFixtureToSchema(t, spec.Name, dir, fixture, published, required)
	}
	if checked < appFixtureFloor {
		t.Fatalf("the sweep checked %d view fixture(s), below the %d floor — the tool-to-view "+
			"derivation has stopped reaching the catalog, which reports PASS having compared nothing",
			checked, appFixtureFloor)
	}
}

// compareFixtureToSchema reports both directions of the drift.
func compareFixtureToSchema(t *testing.T, tool, dir string, fixture map[string]int, published, required map[string]bool) {
	t.Helper()
	for _, member := range slices.Sorted(maps.Keys(fixture)) {
		if envelopeMembers[member] || published[member] {
			continue
		}
		subject := dir + ":" + member
		if fixtureMemberOnlyInTheView.Waived(t, subject) {
			continue
		}
		t.Errorf("%s/fixture.ts carries %q, which %s does not publish.\n"+
			"  The fixture is what the view's suite draws, so a member the tool never answers is a "+
			"panel tested against a payload production does not produce.\n"+
			"  Rename it to what the result struct answers, or ratify it in "+
			"fixtureMemberOnlyInTheView[%q] with the reason the view needs a member the tool does not send.",
			dir, member, tool, subject)
	}
	// The other direction, and only for what the schema REQUIRES. A fixture is
	// one realistic answer, so an optional member it happens not to exercise is
	// a choice rather than drift — reporting those would bury the one finding
	// that matters under every field a payload may omit. A REQUIRED member
	// missing means the fixture is not an answer the tool could have sealed.
	for _, member := range slices.Sorted(maps.Keys(required)) {
		if !published[member] {
			continue
		}
		// AT ITS OWN LEVEL. `required` here is what the tool's shape requires
		// of the payload root, so a member of the same name nested deeper is a
		// different member and must not answer for it.
		if at, carried := fixture[member]; carried && at == dataMemberDepth {
			continue
		}
		t.Errorf("%s REQUIRES %q and %s/fixture.ts does not carry it.\n"+
			"  The fixture claims to be shaped as the tool seals it, so a required member missing from "+
			"it is a payload the tool could not have produced — and the view is tested against it.",
			tool, member, dir)
	}
}

// viewDirOf derives a view's directory from its URI, the way the document's own
// file name is derived from it rather than listed beside it.
func viewDirOf(uri string) string {
	_, file, found := strings.Cut(uri, "ui://margince/")
	if !found {
		return ""
	}
	return strings.TrimSuffix(file, ".html")
}

// schemaMembers collects every property name a schema publishes, at any depth,
// and separately every one a payload must carry.
//
// The two walks differ, and the difference is the whole correctness of the
// second. PUBLISHED is every property anywhere in the document: the fixture is a
// whole payload, so a nested member is as much part of the contract as a
// top-level one, and a fixture member that appears nowhere in the schema is
// drift wherever it sits.
//
// REQUIRED follows only what is reachable THROUGH required members. A `required`
// list inside an optional sub-object says "if you send this object, send these"
// — it does not say the payload must have them. Collecting those flatly reported
// every field of every branch a realistic fixture chose not to exercise, which
// is dozens of findings burying the one that matters.
func toolShapeMembers(t *testing.T, tool string, raw json.RawMessage) (published, required map[string]bool) {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("%s's output schema is not JSON: %v", tool, err)
	}
	// The ADVERTISED schema is the envelope's, with the tool's own shape under
	// `data` (agents/outputshapes.go seals every result). The fixture models the
	// TypeScript Envelope, whose other members are the seal Invoke adds and are
	// deliberately not written by hand — so the subject here is `data` and not
	// the document, or every fixture is reported for lacking a trace id.
	props, isObject := doc["properties"].(map[string]any)
	if !isObject {
		t.Fatalf("%s's output schema publishes no properties, so there is no tool shape to compare "+
			"a fixture against", tool)
	}
	shape, sealed := props[envelopeDataMember]
	if !sealed {
		t.Fatalf("%s's output schema has no %q member — the envelope's shape moved, and this gate is "+
			"comparing fixtures against the wrong half of it", tool, envelopeDataMember)
	}
	published, required = map[string]bool{}, map[string]bool{}
	collectPublished(shape, published)
	collectRequired(shape, required)
	return published, required
}

//craft:ignore naked-any a decoded JSON Schema is an arbitrary document, so the node this walks IS any — naming a type here would describe a shape the deriver is free to change
func collectPublished(node any, out map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if props, isObject := n["properties"].(map[string]any); isObject {
			for name, child := range props {
				out[name] = true
				collectPublished(child, out)
			}
		}
		for key, child := range n {
			if key != "properties" {
				collectPublished(child, out)
			}
		}
	case []any:
		for _, child := range n {
			collectPublished(child, out)
		}
	}
}

// collectRequired descends only through members the payload must carry, and
// stops at an array.
//
// An array's element schema says what a ROW must have, and whether the fixture
// has any rows is not something the schema can say. Descending anyway asserted
// every field of every element type against a fixture that legitimately draws
// `about: []` — seven findings about an empty list, which is the census crying
// wolf at exactly the volume that gets one deleted.
//
// Little is lost, because the direction that catches this ticket's defect is the
// other one: a renamed element member makes the fixture carry a name the schema
// no longer publishes, and that check is total and reaches every depth.
//
//craft:ignore naked-any the same decoded document collectPublished walks, for the same reason
func collectRequired(node any, out map[string]bool) {
	schema, isObject := node.(map[string]any)
	if !isObject {
		return
	}
	props, _ := schema["properties"].(map[string]any)
	if names, hasRequired := schema["required"].([]any); hasRequired {
		for _, name := range names {
			text, isText := name.(string)
			if !isText {
				continue
			}
			out[text] = true
			if child, published := props[text]; published {
				collectRequired(child, out)
			}
		}
	}
}

// fixtureMembers reads the object-literal keys out of a fixture, with the brace
// depth each sits at. Comments and string contents are excluded — a `//` note
// naming a member, or a member name inside a quoted value, is prose.
//
// The DEPTH is what makes the required check mean what it says. Without it both
// sides are flat name sets, and a required member missing from `data` reads as
// present because something nested carries the same name — the exact drift this
// gate exists to catch, hidden by a coincidence of vocabulary. Nesting is
// counted by braces rather than by indentation: a formatter is free to change
// how it indents and is not free to change what nests inside what.
func fixtureMembers(source []byte) map[string]int {
	out := map[string]int{}
	depth := 0
	for _, line := range strings.Split(string(source), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "//") || strings.HasPrefix(trimmed, "*") ||
			strings.HasPrefix(trimmed, "/*") {
			continue
		}
		code := fixtureString.ReplaceAllString(line, `""`)
		if m := fixtureKey.FindStringSubmatch(code); m != nil {
			// The key sits INSIDE the braces open before its line.
			if prior, seen := out[m[1]]; !seen || depth < prior {
				out[m[1]] = depth
			}
		}
		depth += strings.Count(code, "{") + strings.Count(code, "[")
		depth -= strings.Count(code, "}") + strings.Count(code, "]")
	}
	return out
}
