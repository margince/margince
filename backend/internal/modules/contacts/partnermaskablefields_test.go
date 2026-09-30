// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The catalog of what this build can withhold ON A PARTNER, pinned to the code
// that does the withholding.
//
// partnerWithholds is the whole of the partner's. A pair the map holds and the
// catalog does not offer is a mask nobody can configure — the foreign key on
// field_mask refuses the row, so the margin tier stays readable by every seat
// the administrator meant to exclude. A pair the catalog offers and this map
// does not withhold is the mirror: a configuration accepted, stored, read back
// unchanged, and applying to nothing.
//
// The tier's reach onto the commission entry is NOT a second pair. It follows
// from this one through auth's group closure, and offering it separately would
// let an operator set the consequence, believe the tier is hidden, and leave
// the partner reading it.
//
// Only the partner's lines are read: a module may not import a sibling, so each
// object's render test claims its own. gates/maskablefieldobjects_test.go is
// what refuses an object no module claims at all.

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

const partnerMaskableFixture = "../../../migrations/testdata/maskable_fields.txt"

func TestTheMaskableFieldCatalogMatchesWhatThePartnerWithholds(t *testing.T) {
	t.Parallel()

	rendered := renderedPartnerMaskablePairs()
	// An empty render agrees with an empty fixture slice, and both would mean
	// this test stopped reading the map rather than that the partner stopped
	// masking.
	if len(rendered) == 0 {
		t.Fatal("partnerWithholds rendered no pairs — the partner withholds nothing, or this " +
			"test has stopped reading the map it exists to pin")
	}

	want := partnerMaskableFixturePairs(t)
	if strings.Join(rendered, "\n") == strings.Join(want, "\n") {
		return
	}
	t.Errorf("the maskable-field catalog and the code disagree.\n"+
		"partnerWithholds renders:\n  %s\n%s holds:\n  %s\n\n"+
		"Both halves move together or neither does: update the fixture, and add a migration "+
		"writing the same pairs into maskable_field, in this change.",
		strings.Join(rendered, "\n  "), filepath.Base(partnerMaskableFixture), strings.Join(want, "\n  "))
}

// renderedPartnerMaskablePairs is the fixture's own format, rendered from the
// map, so a failure prints something the reader can paste.
func renderedPartnerMaskablePairs() []string {
	pairs := make([]string, 0, len(partnerWithholds))
	for field := range partnerWithholds {
		pairs = append(pairs, partnerMaskObject+" "+field)
	}
	sort.Strings(pairs)
	return pairs
}

// partnerMaskableFixturePairs is the fixture's pairs for partnerMaskObject
// alone.
func partnerMaskableFixturePairs(t *testing.T) []string {
	t.Helper()
	body, err := os.ReadFile(partnerMaskableFixture)
	if err != nil {
		t.Fatalf("reading the maskable-field fixture: %v", err)
	}
	var pairs []string
	for _, line := range strings.Split(string(body), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if object, _, isPair := strings.Cut(line, " "); !isPair || object != partnerMaskObject {
			continue
		}
		pairs = append(pairs, line)
	}
	sort.Strings(pairs)
	return pairs
}
