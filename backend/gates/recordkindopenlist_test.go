// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A column that names a kind of record accepts an extension's kind only when
// a writer puts one there. A column no writer fills with `ext:<unit>:<kind>`
// keeps core's closed list, so the database refuses a row nothing should
// produce. The corpus is the committed head catalog, so a later table is read
// without being listed here.

import (
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

// acceptsExtensionKind are the columns that accept `ext:<unit>:<kind>`, each
// with the writer that puts one there. A listed column whose CHECK refuses the
// form matches nothing, and AssertAllMatched fails it.
var acceptsExtensionKind = gatekit.Waive(map[string]string{
	"list.entity_type":              "collections.Store.CreateList admits a declared kind through acceptsEntityType",
	"list_member.entity_type":       "collections adds members of the list's kind",
	"list_live_member.entity_type":  "collections' live evaluation writes members of the list's kind",
	"list_member_event.entity_type": "collections records each membership change of the list's kind",
	"taggable.entity_type":          "collections.Store.ApplyTag admits a declared kind through acceptsEntityType",
	"saved_view.resource":           "collections saves a view over a declared kind (extvocab.go)",
	"record_grant.record_type":      "row access for extension records is not decided, so the column stays open",
})

func TestOnlyTheNamedColumnsAcceptAnExtensionKind(t *testing.T) {
	t.Parallel()
	defer acceptsExtensionKind.AssertAllMatched(t)
	body := headCatalog(t)
	seen := map[string]bool{}
	for line := range strings.SplitSeq(body, "\n") {
		found := kindListCheck.FindStringSubmatch(line)
		if found == nil || (!strings.Contains(found[3], "'contact'") && !strings.Contains(found[3], "'contacts'")) {
			continue
		}
		subject := found[1] + "." + found[2]
		seen[subject] = true
		if strings.Contains(line, extensionKindPattern) && !acceptsExtensionKind.Waived(t, subject) {
			t.Errorf("%s accepts an extension's `ext:<unit>:<kind>`, and no writer is named for it: keep core's "+
				"closed list, or add the column to acceptsExtensionKind with the writer that fills it.\n\t%s", subject, line)
		}
	}
	if len(seen) < 14 {
		t.Fatalf("the census read %d kind CHECKs, want at least the 14 this gate was written against: the catalog or the pattern changed shape", len(seen))
	}
}
