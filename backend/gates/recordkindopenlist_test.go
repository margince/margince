// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A column that names a kind of record accepts an extension's kind. A closed
// CHECK list is what kept an extension from adding a kind of its own. This
// stops a new column from closing the set again. The corpus is the committed
// head catalog, so a later table is read without being listed here.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// kindListCheck matches a catalog CHECK that restates a closed list of kinds on
// a column that names one.
var kindListCheck = regexp.MustCompile(`^public\.(\w+)\.\w+ CHECK .*?\b(entity_type|object_type|record_type|resource) = ANY \(ARRAY\[(.*?)\]\)`)

// extensionKindPattern is what a CHECK carries to accept an extension kind.
const extensionKindPattern = "^ext:"

// notRecordKinds are the columns whose list is not core's record vocabulary.
var notRecordKinds = gatekit.Waive(map[string]string{
	"activity_link_suggestion.entity_type": "names a core record a message may be filed under, and the relink that an accept runs files under core records only",
	"bulk_operation.record_type":           "names what a bulk change may touch, and `worklist_item` is no record; an extension kind is not offered a bulk change",
})

func TestEveryColumnNamingAKindOfRecordAcceptsAnExtensionKind(t *testing.T) {
	t.Parallel()
	defer notRecordKinds.AssertAllMatched(t)
	body, err := os.ReadFile(filepath.Join(gatekit.ModuleRoot(t), "migrations", "testdata", "head_catalog.txt"))
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	seen := 0
	for line := range strings.SplitSeq(string(body), "\n") {
		found := kindListCheck.FindStringSubmatch(line)
		if found == nil || (!strings.Contains(found[3], "'contact'") && !strings.Contains(found[3], "'contacts'")) {
			continue
		}
		seen++
		if strings.Contains(line, extensionKindPattern) {
			continue
		}
		subject := found[1] + "." + found[2]
		if notRecordKinds.Waived(t, subject) {
			continue
		}
		t.Errorf("%s restates a closed list of record kinds and refuses an extension's `ext:<unit>:<kind>`: "+
			"accept the pattern beside the core words, as the record-kind migration does.\n\t%s", subject, line)
	}
	if seen < 14 {
		t.Fatalf("the census read %d kind CHECKs, want at least the 14 this gate was written against: the catalog or the pattern changed shape", seen)
	}
}
