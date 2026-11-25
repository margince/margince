// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The runner's listing omits `"additionalProperties":false` from every schema it
// renders. The system frame says it once instead, because every step of every
// run re-sends the listing. What it saves is published as catalog.tokens in
// docs/reference/agent-tool-budget.json.
//
// The invariant is held HERE and not only in the runner, because the runner
// cannot see the whole served catalog and this package can. The weaker version
// of this gate — "every omission has a frame sentence" — is in the runner's own
// tests and is not enough on its own: it cannot see a future edit where the
// renderer starts moving schema ordering, enum visibility, required fields or a
// nested member while tools/list stays put. So what is asserted is equivalence:
// for every served spec, applying the declared compaction to the SERVED schema
// equals the schema the listing rendered.

import (
	"bufio"
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// listingSchemaPrefix is how ToolListing introduces a rendered schema.
const listingSchemaPrefix = "  input schema: "

// For every served spec, the listing's schema is the served one with the
// declared omission applied, and nothing else moved.
//
// THE SECOND SIDE IS COMPUTED HERE, not by calling runner.CompactSchema. That is
// the whole strength of this gate. Comparing the listing against the same
// function the listing calls is f(x) == f(x): a mutation INSIDE the compaction —
// `case "enum": continue`, dropping every `description`, dropping `required` —
// moves both sides together and stays green, with only the budget page's byte
// count going red as a "regenerate the page" message a reader obeys.
//
// So this walks the served JSON itself and removes one thing: every
// `"additionalProperties": false` at any depth. Then it compares PARSED TREES,
// so key order and whitespace are the renderer's business and not this gate's.
//
// Derived from the served surface, so a tool added tomorrow is measured without
// anybody remembering this test.
func TestTheListingIsTheDeclaredCompactionOfEveryServedSchema(t *testing.T) {
	specs := servedSurface(t).Specs()
	rendered := renderedListingSchemas(t, specs)
	if len(rendered) != len(specs) {
		t.Fatalf("the listing renders %d schemas for %d served tools — the parse below is reading "+
			"the wrong format, so nothing here is measuring the listing", len(rendered), len(specs))
	}
	for _, spec := range specs {
		got, listed := rendered[spec.Name]
		if !listed {
			t.Errorf("%s is served but the listing renders no schema for it", spec.Name)
			continue
		}
		var listedTree, wantTree any
		if err := json.Unmarshal([]byte(got), &listedTree); err != nil {
			t.Errorf("%s: the listing rendered a schema that is not valid JSON: %v\n%s", spec.Name, err, got)
			continue
		}
		if err := json.Unmarshal(spec.InputSchema, &wantTree); err != nil {
			t.Errorf("%s: the served schema is not valid JSON: %v", spec.Name, err)
			continue
		}
		wantTree = withoutTheDeclaredOmissions(wantTree)
		if !reflect.DeepEqual(listedTree, wantTree) {
			listedJSON, _ := json.Marshal(listedTree)
			wantJSON, _ := json.Marshal(wantTree)
			t.Errorf("%s: the listing renders\n\t%s\nand the served schema minus the two declared "+
				"omissions is\n\t%s\nThe two must be the same tree. If the renderer has started changing "+
				"something else — an enum, a required list, a nested member — that change is invisible to "+
				"mcp-info.md and is paid for on every step of every run.",
				spec.Name, listedJSON, wantJSON)
		}
	}
}

// withoutTheDeclaredOmissions removes, from a parsed schema tree, exactly what
// the listing is allowed to omit — and nothing else.
//
// WHAT IS INDEPENDENT HERE IS THE REMOVAL RULE, not the traversal. The rule is
// written out from the frame's two sentences rather than by calling the
// implementation, which is what stops this gate being f(x) == f(x).
//
// The TRAVERSAL deliberately follows JSON Schema's own structure — `properties`,
// `items`, `additionalProperties` — because that is a fact about the FORMAT, not
// an implementation detail to be independent of. A walk that descended every
// object it found would reach `default`, `const` and `examples`, which hold
// arbitrary INSTANCE data: a tool whose `default` happens to contain
// `{"additionalProperties": false}` would have it stripped here and kept by the
// renderer, and this gate would reject a correct listing. Being independent in
// the wrong dimension turns a gate into a false alarm.
//
//craft:ignore naked-any a decoded JSON document HAS no concrete Go type, and this walk's independence from the implementation's own structure is exactly what makes this side of the gate worth having
func withoutTheDeclaredOmissions(node any) any {
	shape, isObject := node.(map[string]any)
	if !isObject {
		return node
	}
	out := make(map[string]any, len(shape))
	for key, value := range shape {
		switch key {
		case "additionalProperties":
			// The closed form goes; a SCHEMA here is descended into, because it
			// is an object schema like any other.
			if closed, isBool := value.(bool); isBool && !closed {
				continue
			}
			out[key] = withoutTheDeclaredOmissions(value)
		case "properties":
			properties, ok := value.(map[string]any)
			if !ok {
				out[key] = value
				continue
			}
			rewritten := make(map[string]any, len(properties))
			for name, member := range properties {
				rewritten[name] = withoutTheDeclaredOmissions(member)
			}
			out[key] = rewritten
		case "items":
			out[key] = withoutTheDeclaredOmissions(value)
		default:
			// Everything else is carried through UNTOUCHED, including `default`,
			// `const` and `examples`, whose contents are instance data and not
			// schemas.
			out[key] = value
		}
	}
	return out
}

// The compaction actually REMOVES something from the real catalog.
//
// Under-recognition is the one direction this must not fail in: a compaction
// that quietly became a no-op — a renamed reserved member, a keyword this
// surface stopped emitting — would leave the equivalence test above green, the
// saving gone, and no assertion anywhere to notice. So the census counts what it
// took out and refuses to pass on nothing.
func TestTheCompactionStillRemovesWhatTheFrameStatesOnce(t *testing.T) {
	specs := servedSurface(t).Specs()
	closedForms := 0
	for _, spec := range specs {
		closedForms += strings.Count(string(spec.InputSchema), `"additionalProperties":false`)
	}
	if closedForms == 0 {
		t.Error("no served schema carries `\"additionalProperties\":false`, so the compaction has nothing " +
			"to remove and the frame sentence about it is now a claim about nothing")
	}
	// What it removes is gone from the listing at every depth. Several tools
	// close a nested object too.
	if strings.Contains(runner.ToolListing(specs), `"additionalProperties":false`) {
		t.Errorf("the rendered listing still carries the closed form somewhere, so a nested object is "+
			"escaping the compaction (%d occurrences across the served schemas)", closedForms)
	}
}

// The MCP server instructions carry the retry key's rule, and no served schema
// repeats it. The runner's own tests hold its frame to the rule.
func TestTheRetryKeyRuleIsStatedOnceAndNoSchemaRepeatsIt(t *testing.T) {
	keyed := 0
	for _, spec := range servedSurface(t).Specs() {
		if described, spliced := servedRetryKey(t, spec); spliced {
			keyed++
			if described != "" {
				t.Errorf("%s describes the retry key in its own schema, which every write would then "+
					"repeat: %q", spec.Name, described)
			}
		}
	}
	if keyed == 0 {
		t.Fatal("no served schema carries the retry key, so this test asserts nothing about it")
	}
	instructions := servedInstructions(t, primedViews(t, everyDeclaredView()))
	if !strings.Contains(instructions, mcp.ReservedIdempotencyKeyRule) {
		t.Errorf("the MCP server instructions do not state the retry key's rule, and %d schemas carry "+
			"the key with no description:\n%s", keyed, instructions)
	}
}

// servedRetryKey returns the description the SERVED schema gives the retry
// key, and whether the schema carries the member at all.
//
// Parsed rather than pattern-matched: a schema that merely mentions the name in
// its own prose is not a schema that declares the argument.
func servedRetryKey(t *testing.T, spec mcp.ToolSpec) (string, bool) {
	t.Helper()
	var parsed struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &parsed); err != nil {
		t.Errorf("%s: the served input schema is not a JSON object: %v", spec.Name, err)
		return "", false
	}
	member, spliced := parsed.Properties[mcp.ReservedIdempotencyKeyArg]
	return member.Description, spliced
}

// systemFrameBudgetNumerator / Denominator bound the frame at 1/16 of the
// window.
//
// The frame is a PLACE TO PUT THINGS: moving a sentence out of every schema and
// into it trades (tools x sentence) for (1 x sentence), which is a good trade
// and an inviting one, and the catalog floor measures the LISTING alone.
// Without a bound the only thing between a paragraph here and every run of
// every agent paying for it is somebody reading a regenerated doc diff.
//
// Generous on purpose: a ceiling on the shape of the mistake, not a budget to
// argue with. The frame has several times this much room today and still fails
// long before it could displace what a run reasons over.
const (
	systemFrameBudgetNumerator   = 1
	systemFrameBudgetDenominator = 16
)

// The system frame stays a frame.
//
// Fails in the UP direction only — a frame that got shorter is not a defect, and
// pinning the number would make every wording change a test edit.
func TestTheSystemFrameStaysWithinItsShareOfTheWindow(t *testing.T) {
	frame := runner.SystemFrameTokens()
	budget := runner.MinimumPromptWindow * systemFrameBudgetNumerator / systemFrameBudgetDenominator
	if frame > budget {
		t.Errorf("the system frame costs ~%d tokens against the %d this build allows it (%d/%d of "+
			"the window). It is paid on EVERY step of EVERY run and the catalog floor does not "+
			"measure it, so what grows here comes out of the observations a run reasons over. "+
			"Move the rule to the tool whose meaning it is, or argue for the share.",
			frame, budget, systemFrameBudgetNumerator, systemFrameBudgetDenominator)
	}
	// A frame of nothing would mean systemPrompt stopped rendering, which would
	// make every other assertion here vacuous.
	if frame == 0 {
		t.Fatal("the system frame measures zero tokens, so this bound is holding nothing")
	}
}

// renderedListingSchemas parses the schema the listing rendered for each tool.
//
// Read out of ToolListing's real output rather than recomputed, because the
// point is what the model is actually served. A line that does not parse is a
// FAILURE and never a skip: a silently unread line is the shape in which this
// gate would measure a shrinking subset of the catalog and report PASS.
func renderedListingSchemas(t *testing.T, specs []mcp.ToolSpec) map[string]string {
	t.Helper()
	names := map[string]bool{}
	for _, spec := range specs {
		names[spec.Name] = true
	}
	out := map[string]string{}
	var current string
	scanner := bufio.NewScanner(strings.NewReader(runner.ToolListing(specs)))
	// A rendered schema is one line and can be several kilobytes; the default
	// 64KB token limit is comfortable, but the largest served schema is not
	// small and a truncated line would read as a mismatch rather than as a
	// buffer limit.
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		switch {
		case strings.HasPrefix(line, listingSchemaPrefix):
			if current == "" {
				t.Fatalf("a schema line arrived before any tool line, so the listing's format is not "+
					"what this parse assumes: %.80s", line)
			}
			out[current] = strings.TrimPrefix(line, listingSchemaPrefix)
			current = ""
		case strings.HasPrefix(line, "- "):
			name, _, found := strings.Cut(strings.TrimPrefix(line, "- "), " — ")
			if !found || !names[name] {
				t.Fatalf("a tool line names no served tool, so this parse is reading the wrong "+
					"format and would silently measure a subset: %.80s", line)
			}
			current = name
		default:
			t.Fatalf("the listing carries a line this parse does not recognise, which is how a gate "+
				"comes to read less of the catalog than it thinks: %.80s", line)
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("reading the rendered listing: %v", err)
	}
	if current != "" {
		t.Fatalf("%s was listed with no schema line after it", current)
	}
	return out
}
