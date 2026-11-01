// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build poc

package attention

import (
	"os"
	"slices"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/worklist"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// The kind catalog states what today's hand-kept maps state, kind by kind and
// in both directions, so moving the engine onto it changes no behavior.
func TestTheKindCatalogMatchesTodaysEngine(t *testing.T) {
	doc, err := os.ReadFile("../../../api/worklist-kinds.yaml")
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := worklist.LoadCatalog(doc, events.Types())
	if err != nil {
		t.Fatal(err)
	}
	ownerWords := map[ownerKind]string{
		ownerNamed: "named", ownerNobody: "nobody", ownerTheReader: "the_reader",
		ownerFromTheDeal: "from_the_deal", ownerUnreadable: "unreadable",
	}
	destinationWords := map[crmcontracts.WorklistItemDestination]string{
		destinationToday: "today", destinationReview: "review", destinationSystemHealth: "system_health",
	}
	for source := range sourceOwnerAnswers {
		if _, ok := catalog[string(source)]; !ok {
			t.Errorf("%s: the engine knows it, the catalog does not", source)
		}
	}
	mismatches := 0
	for name, kind := range catalog {
		source := crmcontracts.WorklistItemSource(name)
		answers, known := sourceOwnerAnswers[source]
		if !known {
			t.Errorf("%s: the catalog declares it, the engine does not", name)
			continue
		}
		var engineOwners []string
		for k := range answers {
			if word, ok := ownerWords[k]; ok {
				engineOwners = append(engineOwners, word)
			}
		}
		slices.Sort(engineOwners)
		declared := slices.Clone(kind.Owner)
		slices.Sort(declared)
		destination, hasDestination := destinationOfSource[source]
		checks := []struct {
			field, catalog, engine string
		}{
			{"category", kind.Category, string(categoryOfSource(source))},
			{"destination", kind.Destination, map[bool]string{true: destinationWords[destination], false: "none"}[hasDestination]},
			{"owner", strings.Join(declared, ","), strings.Join(engineOwners, ",")},
			{"personal", boolWord(*kind.Personal), boolWord(sourceAnswersForTheActorOnly[source])},
			{"done_in_place", boolWord(*kind.DoneInPlace), boolWord(doneInPlace[source])},
		}
		for _, c := range checks {
			if c.catalog != c.engine {
				mismatches++
				t.Errorf("%s.%s: catalog %q, engine %q", name, c.field, c.catalog, c.engine)
			}
		}
	}
	t.Logf("%d kinds compared on 5 fields each, %d mismatches", len(catalog), mismatches)
}

func boolWord(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
