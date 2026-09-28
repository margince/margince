// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The catalog of what this build can withhold, pinned to the code that does the
// withholding.
//
// fieldmask.Maskable is the whole of it: a mask naming a column with no
// withhold func in dealWithholders is dropped by applyTo, so an administrator
// who configures one has hidden nothing and been told nothing. The database refuses such a row
// outright — field_mask references maskable_field — and this is the half that
// keeps the database's offer equal to what the code actually enforces. A pair
// removed from the map while the table still offers it is a mask an installation
// may configure and no reader applies; a pair added here while the table does
// not offer it is a mask nobody can configure at all.
//
// The fixture is compared against a migrated database by
// compose/integration/maskablefieldparity_integration_test.go, so the three
// stay one statement rather than three that agree by luck.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/fieldmask"
)

const maskableFixture = "../../../migrations/testdata/maskable_fields.txt"

func TestTheMaskableFieldCatalogMatchesWhatTheCodeWithholds(t *testing.T) {
	t.Parallel()

	rendered := renderedMaskablePairs()
	// An empty render agrees with an empty fixture, and both would mean this
	// test stopped reading the map rather than that the deal stopped masking.
	if len(rendered) == 0 {
		t.Fatal("the maskable catalog rendered no pairs — the deal withholds nothing, or this " +
			"test has stopped reading the table it exists to pin")
	}

	want := maskableFixturePairs(t)
	if strings.Join(rendered, "\n") == strings.Join(want, "\n") {
		return
	}
	t.Errorf("the maskable-field catalog and the code disagree.\n"+
		"fieldmask.Maskable(deal) renders:\n  %s\n%s holds:\n  %s\n\n"+
		"Both halves move together or neither does: update the fixture, and add a migration "+
		"writing the same pairs into maskable_field, in this change.",
		strings.Join(rendered, "\n  "), filepath.Base(maskableFixture), strings.Join(want, "\n  "))
}

// renderedMaskablePairs is the fixture's own format, rendered from the map, so
// a failure prints something the reader can paste.
func renderedMaskablePairs() []string {
	maskable := fieldmask.Maskable(maskObject)
	pairs := make([]string, 0, len(maskable))
	for _, field := range maskable {
		pairs = append(pairs, maskObject+" "+field)
	}
	sort.Strings(pairs)
	return pairs
}

// Every field the catalog offers has something here that nulls it. The two
// halves are separate on purpose — the fieldmask package owns WHICH fields a
// mask may name and what each drags with it, the deal owns how a field leaves
// the struct — and a field added to one and forgotten in the other is a mask an
// administrator can configure and no reader applies.
func TestEveryMaskableDealFieldHasSomethingThatWithholdsIt(t *testing.T) {
	t.Parallel()

	for _, field := range fieldmask.Withheld(maskObject, fieldmask.Maskable(maskObject)) {
		if _, withholds := dealWithholders[field]; !withholds {
			t.Errorf("the catalog withholds %q on a deal and dealWithholders has no func for it — "+
				"applyTo drops the name, so the field goes out with its value", field)
		}
	}
	for field := range dealWithholders {
		if !fieldmask.Covers(maskObject, fieldmask.Maskable(maskObject), field) {
			t.Errorf("dealWithholders nulls %q and no mask ever names it — dead code, or a field "+
				"missing from the catalog that nothing can be configured to withhold", field)
		}
	}
}

func maskableFixturePairs(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(maskableFixture)
	if err != nil {
		t.Fatalf("reading the maskable-field fixture: %v", err)
	}
	var pairs []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		pairs = append(pairs, line)
	}
	sort.Strings(pairs)
	return pairs
}
