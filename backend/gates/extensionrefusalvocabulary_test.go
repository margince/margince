// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The refusal vocabulary is spelled on both sides of the wire, so it is ONE
// item.
//
// `extension.RecordRefusal` is what the core writes and what a unit branches
// on; `ExtensionIngestRefusalRefusal` is what the contract publishes to the
// screen that reports it. A class added to one alone is the failure that has no
// symptom: the core records a value the API's enum does not admit, the
// generated client narrows it away, and the card silently stops counting the
// refusals nobody has a name for — which is the same silence the whole
// breadcrumb exists to end.
//
// The schema CHECK is held against the Go set by
// TestEveryDomainEnumMatchesItsSchemaCheck, which binds
// extension_ingest_refusal.refusal to the same type. Between the two, a new
// class fails in three places until it is spelled in all three.
//
// Both sets are DERIVED, neither restated here: a gate holding a list would be
// a third copy of the vocabulary, and the first to go stale.

import (
	"sort"
	"strings"
	"testing"
)

func TestTheRefusalVocabularyIsOneItemOnBothSidesOfTheWire(t *testing.T) {
	t.Parallel()

	published := goConstSet(t, "pkg/extension", "RecordRefusal")
	contract := goConstSet(t, "internal/contracts", "ExtensionIngestRefusalRefusal")

	// A derivation that read nothing agrees with another that read nothing.
	if len(published) == 0 {
		t.Fatal("no extension.RecordRefusal constant found — this gate has stopped reading the " +
			"published vocabulary, and an empty set matches an empty set")
	}
	if len(contract) == 0 {
		t.Fatal("no ExtensionIngestRefusalRefusal constant found — the contract enum is gone or " +
			"regenerated under another name, and this gate cannot see either")
	}

	sort.Strings(published)
	sort.Strings(contract)
	if strings.Join(published, ",") != strings.Join(contract, ",") {
		t.Errorf("the refusal vocabulary disagrees across the wire.\n"+
			"  extension.RecordRefusal (what the core writes): %s\n"+
			"  the contract enum (what the screen reads):      %s\n"+
			"A class only the core knows is one the API narrows away, so the card stops counting "+
			"refusals nobody has a name for. Widen both, and the schema CHECK with them.",
			strings.Join(published, ", "), strings.Join(contract, ", "))
	}
}
