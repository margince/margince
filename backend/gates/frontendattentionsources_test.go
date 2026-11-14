// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every source the Worklist can carry has a row in the frontend's kind table,
// or the client shows a reader the source's own identifier.
//
// The table is generated from api/worklist-kinds.yaml, and KNOWN_SOURCES in
// frontend/src/lib/worklistcopy.ts re-exports it. Reading the generated file
// holds the catalog to the contract on the side the client reads.
//
// The corpus is the contract's own enum, read out of crm.yaml. A source is
// added to the schema first, so a gate reading the generated Go constants
// would agree with a tree nobody had regenerated yet. Both directions are
// compared: a source with no entry fails, and so does an entry naming no source.
//
// The contract is PARSED, not pattern-matched. A first cut read the enum with a
// regex over the raw file and had the one defect a gate must not have: writing
// the same enum block-style instead of inline — an edit that changes nothing
// about the contract — walked the match onto a different schema's vocabulary
// entirely, and it reported a confident wrong answer about a list it was no
// longer reading. Every way the document can be reshaped without changing its
// meaning has to leave this corpus identical, and only a parser gives that.

import (
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	attentionContract    = "api/crm.yaml"
	frontendAttentionMap = "../frontend/src/lib/worklistkinds.gen.ts"
)

// kindTableRow matches a key followed by the opening brace of its fields, so a
// field's own key is never read as a kind.
var kindTableRow = regexp.MustCompile(`(?m)^\s+"?([a-z][a-z0-9_]*)"?:\s*\{`)

// contract is as much of the OpenAPI document as this gate reads: one schema's
// one property's enum. Named down to that so the corpus comes from the path the
// contract actually declares, rather than from whatever the first match in a
// 30,000-line file happened to be.
type contract struct {
	Components struct {
		Schemas struct {
			// The tag spells the schema's own name, which OpenAPI writes in
			// PascalCase. A snake_case tag would match no key in the document
			// and leave this gate reading an empty enum.
			//nolint:tagliatelle // the key is the contract's schema name, not ours to style
			WorklistItem struct {
				Properties struct {
					Source struct {
						Enum []string `yaml:"enum"`
					} `yaml:"source"`
				} `yaml:"properties"`
			} `yaml:"WorklistItem"`
		} `yaml:"schemas"`
	} `yaml:"components"`
}

// contractSources reads the AttentionItem.source enum out of the OpenAPI schema.
func contractSources(t *testing.T) []string {
	t.Helper()
	document, err := gatekit.ReadFile(attentionContract)
	if err != nil {
		t.Fatalf("reading the API contract: %v", err)
	}
	var parsed contract
	if err := yaml.Unmarshal(document, &parsed); err != nil {
		t.Fatalf("parsing the API contract: %v", err)
	}
	sources := parsed.Components.Schemas.WorklistItem.Properties.Source.Enum
	if len(sources) == 0 {
		// Not "no sources" — there is no such contract. The schema was renamed
		// or the property moved, and a gate that shrugged at that would report
		// PASS over a list it had stopped reading.
		t.Fatalf("components.schemas.WorklistItem.properties.source declares no enum in %s: this gate no longer knows what it is guarding", attentionContract)
	}
	slices.Sort(sources)
	return slices.Compact(sources)
}

// surfacedSources reads the kinds out of the generated frontend table that
// KNOWN_SOURCES re-exports. The table is generated from api/worklist-kinds.yaml,
// so this holds the catalog to the contract on the side the client reads.
func surfacedSources(t *testing.T) []string {
	t.Helper()
	source, err := gatekit.ReadFile(frontendAttentionMap)
	if err != nil {
		t.Fatalf("reading the frontend kind table: %v", err)
	}
	const opener = "export const WORKLIST_KINDS = {"
	start := strings.Index(string(source), opener)
	if start < 0 {
		t.Fatalf("no %s declaration in %s", opener, frontendAttentionMap)
	}
	body := string(source)[start+len(opener):]
	if end := strings.Index(body, "\n}"); end >= 0 {
		body = body[:end]
	}
	var sources []string
	for _, match := range kindTableRow.FindAllStringSubmatch(body, -1) {
		sources = append(sources, match[1])
	}
	if len(sources) == 0 {
		t.Fatalf("read no rows out of WORKLIST_KINDS: the parser has stopped seeing the table")
	}
	slices.Sort(sources)
	return slices.Compact(sources)
}

func TestEveryAttentionSourceHasAFocusSurface(t *testing.T) {
	t.Parallel()
	contract := contractSources(t)
	surfaced := surfacedSources(t)

	for _, source := range contract {
		if !slices.Contains(surfaced, source) {
			t.Errorf("source %q can reach the Worklist but has no entry in WORKLIST_KINDS: add it to backend/api/worklist-kinds.yaml and run make gen", source)
		}
	}
	for _, source := range surfaced {
		if !slices.Contains(contract, source) {
			t.Errorf("WORKLIST_KINDS carries %q, which the contract cannot emit: a dead entry teaches the next author the table is not maintained", source)
		}
	}
}
