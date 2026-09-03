// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The runner's listing omits two SURFACE-owned things from every schema it
// renders — `idempotency_key`'s description and `"additionalProperties":false` —
// and states each once in the system frame instead of once per tool. That saves
// ~1,165 tokens on a listing every step of every run re-sends.
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
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/agents/runner"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// listingSchemaPrefix is how ToolListing introduces a rendered schema.
const listingSchemaPrefix = "  input schema: "

// For every served spec, the listing's schema is exactly the compaction applied
// to the served one — nothing else moved.
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
		if want := runner.CompactSchema(spec.InputSchema); got != want {
			t.Errorf("%s: the listing renders\n\t%s\nand the declared compaction of the served schema is\n\t%s\n"+
				"The two must be the same string. If the renderer has started changing something else — "+
				"member order, an enum, a required list, a nested object — that change is invisible to "+
				"mcp-info.md and is being paid for on every step of every run.", spec.Name, got, want)
		}
	}
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
	served := runner.ToolListing(specs)
	var retryDescriptions, closedForms int
	// The retry key's SERVED description, read out of the real catalog rather
	// than typed here. A literal would search for a string the surface no longer
	// writes the moment somebody rewords it — passing on nothing, which is the
	// one direction a census must not fail in.
	var retryDescription string
	for _, spec := range specs {
		schema := string(spec.InputSchema)
		if described := servedRetryKeyDescription(t, spec); described != "" {
			retryDescriptions++
			if retryDescription == "" {
				retryDescription = described
			} else if described != retryDescription {
				t.Errorf("%s describes the retry key differently from its siblings; the surface has "+
					"ONE definition of that member and the frame states ONE rule:\n  %s\n  %s",
					spec.Name, retryDescription, described)
			}
		}
		closedForms += strings.Count(schema, `"additionalProperties":false`)
	}
	if retryDescriptions == 0 {
		t.Error("no served schema carries the retry key with a description, so the first half of the " +
			"compaction has nothing to remove — either the surface stopped splicing the member, or the " +
			"name this reads has been renamed out from under it")
	}
	if closedForms == 0 {
		t.Error("no served schema carries `\"additionalProperties\":false`, so the second half of the " +
			"compaction has nothing to remove and the frame sentence about it is now a claim about " +
			"nothing")
	}
	// And what it removes is GONE from the listing, at every depth: the
	// whole-catalog count of the closed form exceeds the tool count because
	// several tools close a nested object too.
	if strings.Contains(served, `"additionalProperties":false`) {
		t.Errorf("the rendered listing still carries the closed form somewhere, so a nested object is "+
			"escaping the compaction (%d occurrences across the served schemas)", closedForms)
	}
	if strings.Contains(served, retryDescription) {
		t.Errorf("the rendered listing still carries the retry key's served description, which the "+
			"surface defines once and the frame states once (%d tools splice it): %s",
			retryDescriptions, retryDescription)
	}
	// The other end of the same invariant: the catalogue's description and the
	// frame's sentence come from ONE constant. If they ever diverge, the checks
	// above go looking for text nothing writes and pass on nothing.
	if !strings.Contains(retryDescription, mcp.ReservedIdempotencyKeyRule) {
		t.Errorf("the served description %q does not carry mcp.ReservedIdempotencyKeyRule, so the "+
			"catalogue and the frame are stating one rule from two sources again", retryDescription)
	}
}

// servedRetryKeyDescription returns the description the SERVED schema gives the
// retry key, or "" when this tool does not carry the member.
//
// Parsed rather than pattern-matched: the member's presence and its description
// are two different facts, and a schema that merely mentions the name somewhere
// in its own prose is not a schema that declares the argument.
func servedRetryKeyDescription(t *testing.T, spec mcp.ToolSpec) string {
	t.Helper()
	var parsed struct {
		Properties map[string]struct {
			Description string `json:"description"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(spec.InputSchema, &parsed); err != nil {
		t.Errorf("%s: the served input schema is not a JSON object: %v", spec.Name, err)
		return ""
	}
	return parsed.Properties[mcp.ReservedIdempotencyKeyArg].Description
}

// tools/list must NOT move. The compaction is the runner's rendering, and a
// client's catalogue is the thing mcp-info.md is the check on — so a served
// schema keeps every byte the compaction takes out of the listing.
//
// Asserted here as well as by the mcp-info drift gate, because that gate fails
// with "the page is stale" and a reader regenerates the page. This one says
// which invariant broke.
func TestTheServedSurfaceKeepsWhatTheListingOmits(t *testing.T) {
	var retryKeyed, closed int
	for _, spec := range servedSurface(t).Specs() {
		var parsed struct {
			Properties map[string]struct {
				Description string `json:"description"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &parsed); err != nil {
			t.Errorf("%s: the served input schema is not a JSON object: %v", spec.Name, err)
			continue
		}
		if member, spliced := parsed.Properties[mcp.ReservedIdempotencyKeyArg]; spliced {
			retryKeyed++
			if member.Description == "" {
				t.Errorf("%s: the SERVED schema's retry key carries no description — the compaction "+
					"has leaked into tools/list, where a client holds this catalogue for a whole "+
					"session and has no frame to read the rule from", spec.Name)
			}
		}
		closed += strings.Count(string(spec.InputSchema), `"additionalProperties":false`)
	}
	if retryKeyed == 0 || closed == 0 {
		t.Fatalf("the served surface splices the retry key into %d tools and closes %d schemas; "+
			"with either at zero this test asserts nothing about tools/list", retryKeyed, closed)
	}
}

// systemFrameBudgetNumerator / Denominator bound the frame at 1/16 of the
// window — 1,536 tokens against today's 351.
//
// The bound exists because this change made the frame a PLACE TO PUT THINGS.
// Moving a sentence out of 32 schemas and into the frame trades (tools x
// sentence) for (1 x sentence), which is a good trade and an inviting one, and
// the catalog floor measures the LISTING alone. Without a bound the only thing
// between a paragraph in the frame and every run of every agent paying for it
// is somebody noticing a regenerated doc diff.
//
// Generous on purpose: it is a ceiling on the shape of the mistake, not a
// budget to argue with. Four times today's frame still fails long before it
// could displace the observations a run reasons over.
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
	budget := runner.PromptTokenCeiling * systemFrameBudgetNumerator / systemFrameBudgetDenominator
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
