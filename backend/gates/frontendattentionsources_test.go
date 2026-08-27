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

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"
)

const (
	attentionContract      = "api/crm.yaml"
	frontendAttentionMap   = "../frontend/src/screens/attentionsource.ts"
	attentionItemSchemaKey = "\n    AttentionItem:"
)

// sourceEnumLine reads the inline enum off the `source` property.
var sourceEnumLine = regexp.MustCompile(`(?m)^\s*enum:\s*\[([^\]]*)\]`)

// focusSurfaceEntry reads one `source: "surface"` pair out of the map.
var focusSurfaceEntry = regexp.MustCompile(`["']?\b([a-z][a-z0-9_]*)\b["']?:\s*["']([a-z]+)["']`)

// tsCommentInSources strips comments before the entries are read: the map is
// commented and the prose names sources, so a source deleted from the map but
// mentioned above it would keep this gate green.
var tsCommentInSources = regexp.MustCompile(`(?s)//[^\n]*|/\*.*?\*/`)

// contractSources reads the AttentionItem.source enum out of the OpenAPI schema.
//
// Scoped to the AttentionItem schema rather than the whole document: crm.yaml
// holds hundreds of inline enums, and the first one a file-wide scan met would
// be some other schema's vocabulary entirely.
func contractSources(t *testing.T) []string {
	t.Helper()
	document, err := os.ReadFile(attentionContract)
	if err != nil {
		t.Fatalf("reading the API contract: %v", err)
	}
	start := strings.Index(string(document), attentionItemSchemaKey)
	if start < 0 {
		t.Fatalf("no %s schema in %s — this gate is reading the wrong document", attentionItemSchemaKey, attentionContract)
	}
	schema := string(document)[start:]
	property := strings.Index(schema, "\n        source:")
	if property < 0 {
		t.Fatalf("AttentionItem declares no source property: this gate no longer knows what it is guarding")
	}
	match := sourceEnumLine.FindStringSubmatch(schema[property:])
	if match == nil {
		t.Fatalf("AttentionItem.source carries no inline enum: the corpus this gate derives has moved")
	}
	var sources []string
	for _, value := range strings.Split(match[1], ",") {
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			sources = append(sources, trimmed)
		}
	}
	if len(sources) == 0 {
		t.Fatalf("read no values out of the AttentionItem.source enum — the parser has stopped seeing it")
	}
	slices.Sort(sources)
	return slices.Compact(sources)
}

// surfacedSources reads the FOCUS_SURFACE literal out of the TypeScript module.
func surfacedSources(t *testing.T) []string {
	t.Helper()
	source, err := os.ReadFile(frontendAttentionMap)
	if err != nil {
		t.Fatalf("reading the frontend focus-surface map: %v", err)
	}
	const opener = "export const FOCUS_SURFACE"
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
		t.Fatalf("read no entries out of FOCUS_SURFACE — the parser has stopped seeing the map")
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
