// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every core filter field the engine can name has a word on the builder screen.
//
// The vocabulary read sends wire names — `owner_id` — and deliberately carries
// no label, so the filter builder keeps its own map from name to catalog key.
// A field added to an engine without a word there reached readers as "owner
// id"; a word left behind for a field the engine dropped looks like coverage
// and names nothing. Both halves are read from their owners here: the engine
// set from collections' SegmentEngine over every resource the contract admits,
// the screen's set from the map literal itself.

import (
	"context"
	"os"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
)

const frontendFilterFieldLabels = "../frontend/src/screens/filterdata.ts"

// filterLabelEntry reads one field name opening a `[name, key]` pair, in either
// quote style, so an entry written the other way cannot vanish from the census.
var filterLabelEntry = regexp.MustCompile(`\[\s*["']([a-z0-9_]+)["']\s*,`)

func TestEveryCoreFilterFieldHasAWordOnTheBuilder(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendFilterFieldLabels)
	if err != nil {
		t.Fatalf("reading the filter builder's labels: %v", err)
	}
	const opener = "FILTER_FIELD_LABELS = new Map<string, MessageKey>(["
	start := indexAfter(string(source), opener)
	if start < 0 {
		t.Fatalf("%s no longer declares FILTER_FIELD_LABELS as a Map literal — this gate is reading a shape that is gone",
			frontendFilterFieldLabels)
	}
	end := indexAfter(string(source)[start:], "]);")
	if end < 0 {
		t.Fatalf("%s's FILTER_FIELD_LABELS literal is unterminated", frontendFilterFieldLabels)
	}
	literal := tsProviderComment.ReplaceAllString(string(source)[start:start+end], " ")
	labelled := map[string]bool{}
	for _, m := range filterLabelEntry.FindAllStringSubmatch(literal, -1) {
		labelled[m[1]] = true
	}

	engine := map[string]bool{}
	doc := loadContractDocument(t)
	for _, resource := range vocabularyResources(t, doc) {
		query, ok, err := (&collections.Store{}).SegmentEngine(context.Background(), resource)
		if err != nil {
			t.Fatalf("segment engine for %s: %v", resource, err)
		}
		if !ok {
			t.Errorf("the contract admits resource %q and no engine serves it", resource)
			continue
		}
		for name := range query.Fields {
			engine[name] = true
		}
	}
	if len(engine) == 0 {
		t.Fatal("no engine named a field, so this gate compared nothing")
	}

	for _, name := range sortedKeys(engine) {
		if !labelled[name] {
			t.Errorf("the filter engine names %q and %s gives it no word, so the builder shows its wire name to a reader",
				name, frontendFilterFieldLabels)
		}
	}
	for _, name := range sortedKeys(labelled) {
		if !engine[name] {
			t.Errorf("%s labels %q and no filter engine has that field — a stale word that reads as coverage",
				frontendFilterFieldLabels, name)
		}
	}
}
