// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// Every source the Worklist can carry must have a body the decision lane knows
// how to draw, or the lane asks the wrong endpoint about it.
//
// The decision lane used to branch on one source and treat everything else as a
// staged proposal, fetching `/approvals/{id}` with the item's own id. That read
// answers 404 for an item that is not an approval, so the card rendered as a
// failed one — on the single surface whose promise is that it can be finished.
// Nothing failed when a source was added, because the assumption was spelled as
// an `else` rather than as a case.
//
// The corpus is the contract's own enum, read out of crm.yaml rather than out of
// the generated Go constants: a source is added to the schema first, and a gate
// that read the generated side would agree with a tree nobody had regenerated
// yet. Both directions are compared — a source with no entry fails, and an entry
// naming no source fails too, because a dead entry teaches the next author that
// the map is not maintained.
//
// The contract is PARSED, not pattern-matched. A first cut read the enum with a
// regex over the raw file and had the one defect a gate must not have: writing
// the same enum block-style instead of inline — an edit that changes nothing
// about the contract — walked the match onto a different schema's vocabulary
// entirely, and it reported a confident wrong answer about a list it was no
// longer reading. Every way the document can be reshaped without changing its
// meaning has to leave this corpus identical, and only a parser gives that.

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

const (
	attentionContract    = "api/crm.yaml"
	frontendAttentionMap = "../frontend/src/screens/worklist.copy.ts"
)

// focusSurfaceEntry reads one `source: "surface"` pair out of the map.
var focusSurfaceEntry = regexp.MustCompile(`["']?\b([a-z][a-z0-9_]*)\b["']?:\s*(?:["'][a-z]+["']|true)`)

// tsCommentInSources strips comments before the entries are read: the map is
// commented and the prose names sources, so a source deleted from the map but
// mentioned above it would keep this gate green.
var tsCommentInSources = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

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
	document, err := os.ReadFile(attentionContract)
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

// surfacedSources reads the KNOWN_SOURCES literal out of the TypeScript module.
//
// The map moved when the lane page became the ranked queue: the decision lane's
// FOCUS_SURFACE chose which BODY to draw, and this one decides whether a source
// has a sentence at all. The invariant is the same either way — a source the
// contract can emit and the client cannot name reaches a reader as an
// identifier — so the gate follows the map rather than being deleted with the
// page it used to guard.
func surfacedSources(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(frontendAttentionMap)
	if err != nil {
		t.Fatalf("reading the frontend focus-surface map: %v", err)
	}
	const opener = "const KNOWN_SOURCES = {"
	start := strings.Index(string(source), opener)
	if start < 0 {
		t.Fatalf("no %s declaration in %s", opener, frontendAttentionMap)
	}
	body := string(source)[start:]
	if end := strings.Index(body, "\n}"); end >= 0 {
		body = body[:end]
	}
	body = tsCommentInSources.ReplaceAllString(body, "")
	var sources []string
	for _, match := range focusSurfaceEntry.FindAllStringSubmatch(body, -1) {
		sources = append(sources, match[1])
	}
	if len(sources) == 0 {
		t.Fatalf("read no entries out of KNOWN_SOURCES — the parser has stopped seeing the map")
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
			t.Errorf("source %q can reach the Worklist but has no entry in FOCUS_SURFACE: the decision lane would read /approvals with an id that is not an approval's", source)
		}
	}
	for _, source := range surfaced {
		if !slices.Contains(contract, source) {
			t.Errorf("FOCUS_SURFACE carries %q, which the contract cannot emit: a dead entry teaches the next author the map is not maintained", source)
		}
	}
}
