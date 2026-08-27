// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

import (
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/people"
)

// The dedupe evidence snapshot is stored as free JSON, so nothing about a field
// name is checked when it is written. The client turns each name into a phrase
// in the reader's own language, which makes the two lists one invariant spelled
// on both sides of a wire.
//
// This fails in BOTH directions on purpose. A field the detector writes and the
// contract omits reaches a person as a database column — the leak that put
// `full_name` and `org` on screen. A field the contract publishes and nothing
// writes is a word three translations carry for a row that never arrives.
func TestEveryDedupeEvidenceFieldIsNameableOnTheWire(t *testing.T) {
	t.Parallel()
	written := people.DedupeEvidenceFields()
	published := publishedEvidenceEnum(t, "field")

	for _, field := range written {
		if !slices.Contains(published, field) {
			t.Errorf("the detector writes evidence field %q and the contract does not publish it; "+
				"a client cannot name it, so it reaches a reader as a column name", field)
		}
	}
	for _, field := range published {
		if !slices.Contains(written, field) {
			t.Errorf("the contract publishes evidence field %q and nothing writes it; "+
				"every locale carries a phrase for a row that never arrives", field)
		}
	}
}

// The same obligation for the detector's verdicts.
//
// Split from the fields on purpose: an evidence row is dropped when EITHER half
// is unrecognised, so a gate that reads only the field names reports PASS while
// every identity-conflict card on screen shows an empty comparison. That is
// exactly what it did.
func TestEveryDedupeEvidenceSignalIsNameableOnTheWire(t *testing.T) {
	t.Parallel()
	written := people.DedupeEvidenceSignals()
	published := publishedEvidenceEnum(t, "signal")

	for _, signal := range written {
		if !slices.Contains(published, signal) {
			t.Errorf("the detector writes evidence signal %q and the contract does not publish it; "+
				"every row carrying it is dropped, and a pair whose only row it is arrives unexplained", signal)
		}
	}
	for _, signal := range published {
		if !slices.Contains(written, signal) {
			t.Errorf("the contract publishes evidence signal %q and nothing writes it; "+
				"every locale carries a phrase for a row that never arrives", signal)
		}
	}
}

// publishedEvidenceEnum reads one property's enum out of the contract rather
// than restating it. A test carrying its own copy of the list it guards is a
// second copy of the thing that drifted.
//
// The property is named rather than taken positionally. An earlier version took
// the FIRST enum after the schema anchor, which happened to be the right one
// only because `field` is declared above `signal` — reordering two properties
// in the contract would have left this gate comparing field names against
// verdicts and reporting confident nonsense.
func publishedEvidenceEnum(t *testing.T, property string) []string {
	t.Helper()
	source, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	const anchor = "    AttentionPairEvidence:"
	start := strings.Index(string(source), anchor)
	if start < 0 {
		t.Fatalf("AttentionPairEvidence is not in the contract; this gate is guarding nothing")
	}
	block := string(source)[start:]
	// The property's own line, then the first enum under it.
	propAt := strings.Index(block, "\n        "+property+":\n")
	if propAt < 0 {
		t.Fatalf("AttentionPairEvidence declares no %q property; this gate is guarding nothing", property)
	}
	match := regexp.MustCompile(`enum: \[([^\]]+)\]`).FindStringSubmatch(block[propAt:])
	if match == nil {
		t.Fatalf("AttentionPairEvidence.%s publishes no enum; this gate is guarding nothing", property)
	}
	values := strings.Split(match[1], ",")
	for i := range values {
		values[i] = strings.TrimSpace(values[i])
	}
	slices.Sort(values)
	return values
}
