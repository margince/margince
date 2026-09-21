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

// viewsWithoutAFixture ratifies a view that models no payload, keyed by the
// tool that carries it.
//
// Declared rather than skipped. A missing fixture is otherwise indistinguishable
// from a deleted one, and deleting a fixture would drop its view out of this
// check with nothing to notice — a census counting down in silence.
var viewsWithoutAFixture = gatekit.Waive(map[string]string{
	"check_location_support": "the geo probe renders no record and answers no tool's result: it reports whether THIS host lets a view read the device's position, which is a fact about the client rather than a payload. There is no shape for a fixture to model. It is also meant to be deleted once the host matrix is filled in, which its own catalog entry says",
})

// appFixtureFloor is the number of views the sweep must find. Below it the
// derivation has stopped reaching the catalog, and a sweep that judges nothing
// reports PASS.
const appFixtureFloor = 4

// fixtureKey matches one member of a JavaScript object literal. This tree's
// formatter puts one key per line, which is what makes a line-wise read of a
// fixture complete rather than approximate — and appFixtureFloor plus the
// per-fixture count below are what fail if that ever stops being true.
var fixtureKey = regexp.MustCompile(`^\s*([a-z][A-Za-z0-9_]*)\s*:`)

func TestEveryAppViewFixtureMatchesItsToolsOutputSchema(t *testing.T) {
	t.Parallel()
	defer fixtureMemberOnlyInTheView.AssertAllMatched(t)
	defer viewsWithoutAFixture.AssertAllMatched(t)

	checked := 0
	for _, spec := range compose.NewRegistry(nil, compose.SendPath{}).Specs() {
		if spec.UI == nil || spec.OutputSchema == nil {
			continue
		}
		dir := viewDirOf(spec.UI.ResourceURI)
		path := appFixtureDir + "/" + dir + "/fixture.ts"
		source, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			// A FINDING, not a skip. A tool carrying both a view and an output
			// schema is one whose payload a fixture is supposed to model, and
			// skipping the missing one means deleting a fixture silently drops
			// its view out of the check — with the floor still met by the
			// others, which is a census counting down without saying so.
			//
			// The geo probe does not reach here: it declares no output schema
			// and is excluded above, because it answers no tool's result.
			if !viewsWithoutAFixture.Waived(t, spec.Name) {
				t.Errorf("%s carries the view at %s and declares an output schema, but %s does not "+
					"exist — a view whose payload nothing models is one no test has drawn the real "+
					"shape of. Add the fixture, or ratify the view in viewsWithoutAFixture[%q]",
					spec.Name, spec.UI.ResourceURI, path, spec.Name)
			}
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
		if _, carried := fixture[member]; carried {
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
	collectPublished(shape, nil, published)
	collectRequired(shape, nil, required)
	return published, required
}

//craft:ignore naked-any a decoded JSON Schema is an arbitrary document, so the node this walks IS any — naming a type here would describe a shape the deriver is free to change
func collectPublished(node any, at []string, out map[string]bool) {
	switch n := node.(type) {
	case map[string]any:
		if props, isObject := n["properties"].(map[string]any); isObject {
			for name, child := range props {
				here := append(append([]string{}, at...), name)
				out[strings.Join(here, ".")] = true
				collectPublished(child, here, out)
			}
		}
		// An array contributes no segment, matching the fixture side: a row's
		// members belong to the collection rather than to an index.
		collectPublished(n["items"], at, out)
	case []any:
		for _, child := range n {
			collectPublished(child, at, out)
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
func collectRequired(node any, at []string, out map[string]bool) {
	schema, isObject := node.(map[string]any)
	if !isObject {
		return
	}
	props, _ := schema["properties"].(map[string]any)
	names, hasRequired := schema["required"].([]any)
	if !hasRequired {
		return
	}
	for _, name := range names {
		text, isText := name.(string)
		if !isText {
			continue
		}
		here := append(append([]string{}, at...), text)
		out[strings.Join(here, ".")] = true
		if child, published := props[text]; published {
			collectRequired(child, here, out)
		}
	}
}

// fixtureMembers reads the object-literal keys out of a fixture, each with the
// PATH it sits at — "items.deal_id" rather than "deal_id".
//
// Paths and not names, because a name alone loses which member is meant. The
// handoff fixture carries a root `name` and a nested `deals[].name`: with names,
// a schema that dropped the root one while keeping the nested one leaves the
// stale root field reading as published, which is the drift this gate is for.
// Arrays contribute no segment — a row's members belong to the collection, and
// an index would make every fixture's second row a different path.
//
// Scanned rather than regex-stripped. A quoted value may contain an escaped
// quote or a brace, and `"[^"]*"` ends the string at the wrong place for the
// first and lets the second through — either one corrupts the nesting from that
// line on and reports valid fixtures as missing required members. Comments are
// skipped the same way, mid-line ones included.
func fixtureMembers(source []byte) map[string]int {
	out := map[string]int{}
	var path []string
	for _, line := range strings.Split(string(source), "\n") {
		code := fixtureCode(line)
		if key := fixtureKey.FindStringSubmatch(code); key != nil {
			if at, under := underData(append(append([]string{}, path...), key[1])); under {
				out[at] = len(path)
			}
		}
		// The key is recorded at the depth it OPENS at, so nesting is updated
		// after it: `items: [` names items at this level and its rows below.
		for _, r := range code {
			switch r {
			case '{', '[':
				path = append(path, lastKeyOn(code))
			case '}', ']':
				if len(path) > 0 {
					path = path[:len(path)-1]
				}
			}
		}
	}
	return out
}

// underData answers a fixture path relative to the envelope's `data`, and
// whether it is under it at all.
//
// The fixture models an Envelope, so every path starts inside the literal's
// outermost brace (an anonymous segment) and then under `data`. The schema side
// is relative to the tool's own shape, so the two only line up once that prefix
// is off. A path outside `data` — the envelope's `warnings` — is not the tool's
// and answers false.
func underData(path []string) (string, bool) {
	named := make([]string, 0, len(path))
	for _, segment := range path {
		if segment != "" {
			named = append(named, segment)
		}
	}
	if len(named) == 0 || named[0] != envelopeDataMember {
		return "", false
	}
	return strings.Join(named[1:], "."), len(named) > 1
}

// lastKeyOn is the member a brace on this line opens under, or "" for an
// anonymous one — an array's element object, which contributes no segment.
func lastKeyOn(code string) string {
	if key := fixtureKey.FindStringSubmatch(code); key != nil {
		return key[1]
	}
	return ""
}

// fixtureCode is one line with its comments and string CONTENTS removed, so
// neither can be read as structure or as a key.
//
// A hand-rolled scan because the alternatives are wrong in ways that matter
// here: a regex for a quoted run cannot express "unless the quote is escaped",
// and a JSON parser cannot read a TypeScript literal at all.
func fixtureCode(line string) string {
	var out strings.Builder
	var quote rune
	escaped := false
	runes := []rune(line)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case quote != 0 && escaped:
			escaped = false
		case quote != 0 && r == '\\':
			escaped = true
		case quote != 0 && r == quote:
			quote = 0
			out.WriteRune('"')
		case quote != 0:
			// Inside a string: contributes nothing, whatever it is.
		case r == '"' || r == '\'' || r == '`':
			quote = r
			out.WriteRune('"')
		case r == '/' && i+1 < len(runes) && (runes[i+1] == '/' || runes[i+1] == '*'):
			// A comment starts here and this reader is line-wise, so the rest
			// of the line is prose. A block comment's later lines carry no
			// key and no brace this cares about — and if one ever did, the
			// per-fixture count below is what notices the read going wrong.
			return out.String()
		default:
			out.WriteRune(r)
		}
	}
	return out.String()
}
