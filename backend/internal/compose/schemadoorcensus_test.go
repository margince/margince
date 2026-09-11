// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every `margince://schema/*` document this surface publishes has a TOOL that
// answers it.
//
// mcppointers_test.go holds the other direction — a document a tool NAMES is one
// the surface publishes — and its helpers are reused here rather than copied,
// because both censuses ask the same two corpora the same question and differ
// only in what they conclude.
//
// A resource is reachable by an MCP client that reads resources and by nobody
// else. Most callers here list TOOLS only, and the Surface-B runner is offered
// no resource step at all, so for them a document with no door is reachable by
// refusal alone. The refusals are good — every one names the offending key and
// returns the accepted list — and they are not free: a tools-only run writing an
// organization, a person and a relationship from one business card spent one
// turn per record type being told the field names it had guessed, because
// margince://schema/record-fields was published with no door while the four
// documents around it had one.
//
// BOTH SIDES ARE DERIVED. The documents come from the composed resource
// providers, so a sixth one wired tomorrow inherits the obligation without
// anybody remembering; the doors come from the served tool specs, so a door
// withdrawn goes red rather than leaving a list that still claims it.
//
// SCOPED TO `schema/`, and the scope is a property of the subject rather than a
// skip list: a schema document is the vocabulary some verb refuses against, and
// a tools-only caller can reach it no other way. margince://capabilities is the
// one published document outside that prefix, and it needs no door because it
// describes the TOOL SURFACE — a client that lists tools has already been served
// what it holds.
//
// There is deliberately no exemption map. A document owing no door would be one
// no verb refuses against, which is a design question for a reviewer, not a line
// somebody adds to get green.
//
// It lives in compose because only compose has the whole surface: a registry
// built in the agents package has no tools in it, and the providers are composed
// here from three modules, so a census in either module would walk a fragment
// and report PASS.

import (
	"encoding/json"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// schemaDocumentPrefix is what makes a published document a VOCABULARY rather
// than a description of the surface itself.
const schemaDocumentPrefix = "margince://schema/"

func TestEveryPublishedSchemaDocumentHasAToolThatAnswersIt(t *testing.T) {
	specs := servedSurface(t).Specs()
	if len(specs) == 0 {
		t.Fatal("the served surface offered no tools, so this census walked nothing")
	}
	doors := namedDocuments(documentDoors(t, specs))
	if len(doors) == 0 {
		t.Fatal("no served tool answers any document, so this census found no doors at all — " +
			"the predicate no longer recognises one")
	}
	checked := 0
	for uri := range publishedDocumentURIs(t) {
		if !strings.HasPrefix(uri, schemaDocumentPrefix) {
			continue
		}
		checked++
		switch answering := doors[uri]; len(answering) {
		case 1:
		case 0:
			t.Errorf("%s is published as a resource and no tool answers it, so a client that lists "+
				"tools and reads no resources reaches that vocabulary only by being refused. "+
				"Register a describe tool serving the same document.", uri)
		default:
			sort.Strings(answering)
			t.Errorf("%s is answered by %s — two doors onto one document are two answers to one "+
				"question, and they drift apart. Serve it from one tool.",
				uri, strings.Join(answering, " and "))
		}
	}
	// A census that stops seeing the documents reports PASS in the same words as
	// one with nothing left to fix. The lower bound is the tool surface's own
	// pointers, read independently: a provider dropped from the composition
	// still leaves the verbs that refuse against its document naming it.
	if named := schemaDocumentsNamedByTools(specs); checked < named {
		t.Fatalf("%d schema documents were read off the resource surface while the tool surface "+
			"names %d — the walk has lost a provider", checked, named)
	}
}

// documentDoors narrows the surface to the tools that SERVE a document rather
// than refuse against one.
//
// Taking no arguments is the difference, and it is structural rather than a name
// convention: run_report and create_record both name a vocabulary, and both take
// the arguments that vocabulary describes. A tool that took none and merely
// mentioned a URI would surface as a SECOND door and fail the census rather than
// covering for a missing one.
func documentDoors(t *testing.T, specs []mcp.ToolSpec) []mcp.ToolSpec {
	t.Helper()
	var doors []mcp.ToolSpec
	for _, spec := range specs {
		var declared struct {
			Properties map[string]json.RawMessage `json:"properties"`
		}
		if err := json.Unmarshal(spec.InputSchema, &declared); err != nil {
			t.Fatalf("%s advertises an input schema no client can parse: %v", spec.Name, err)
		}
		if len(declared.Properties) == 0 {
			doors = append(doors, spec)
		}
	}
	return doors
}

// schemaDocumentsNamedByTools counts the distinct vocabularies the tool surface
// points a caller at, whether or not anything answers them.
func schemaDocumentsNamedByTools(specs []mcp.ToolSpec) int {
	named := 0
	for uri := range namedDocuments(specs) {
		if strings.HasPrefix(uri, schemaDocumentPrefix) {
			named++
		}
	}
	return named
}
