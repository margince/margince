// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// Every core filter field the engine can name has a word on the builder screen.
//
// The vocabulary read sends wire names — `owner_id` — and deliberately carries
// no label. The builder takes a field's word from the History tab's contract
// map first and from its own filter-only map second, so the union of the two
// must cover every engine field. A field added to an engine without a word
// reached readers as "owner id"; a filter-only word for a field the engine
// dropped looks like coverage and names nothing; and a filter-only word for a
// field History already names is a second spelling the lookup never reads.
// Every set is read from its owner: the engine's from collections'
// SegmentEngine over every resource the contract admits, the words from the
// map literals themselves.

import (
	"context"
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/modules/collections"
)

const (
	frontendFilterFieldLabels = "../frontend/src/screens/filterdata.ts"
	filterOnlyLabelMap        = "FILTER_ONLY_FIELD_LABELS = new Map<string, MessageKey>(["
)

func TestEveryCoreFilterFieldHasAWordOnTheBuilder(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(frontendFilterFieldLabels)
	if err != nil {
		t.Fatalf("reading the filter builder's labels: %v", err)
	}
	filterOnly := tsMapKeys(t, frontendFilterFieldLabels, string(source), filterOnlyLabelMap)
	history, _ := historyLabelMaps(t)

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
		if !history[name] && !filterOnly[name] {
			t.Errorf("the filter engine names %q and neither %s nor %s gives it a word, so the builder shows its wire name to a reader",
				name, historyLabelSource, frontendFilterFieldLabels)
		}
	}
	for _, name := range sortedKeys(filterOnly) {
		if !engine[name] {
			t.Errorf("%s labels %q and no filter engine has that field — a stale word that reads as coverage",
				frontendFilterFieldLabels, name)
		}
		if history[name] {
			t.Errorf("%s labels %q, which %s already names — the builder reads History's word first, so this one is a second spelling nothing reads",
				frontendFilterFieldLabels, name, historyLabelSource)
		}
	}
}
