// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// An activity is correlated to its stored original in one place.
//
// storekit.StoredOriginalSQL reads the link column first and the natural key
// only where there is no link. A copy that kept the bare key join stops
// reaching every Telegram original, because the poll and the activity key the
// same message differently. Retention kept that join, and its Telegram
// originals never aged out.
//
// The fallback arm is what a copy has to spell: an alias's raw_capture_id
// tested for NULL. So the gate reads every hand-written Go file for that shape.
//
// It cannot see a bare key join that skips the link column. That join is
// exact for a path scoped to mail or calendar, whose originals share the
// activity's key. The tree holds a dozen of them.

import (
	"regexp"
	"testing"
)

// storedOriginalFile is the one file allowed to spell the fallback arm.
const storedOriginalFile = "internal/platform/database/storekit/storedoriginal.go"

// storedOriginalFallback is the arm a copy of the correlation needs: an
// alias-qualified raw_capture_id tested for NULL. The owner's format verb
// counts as an alias.
var storedOriginalFallback = regexp.MustCompile(`(\w+|%\[\d\]s)\.raw_capture_id\s+IS\s+NULL`)

func TestTheStoredOriginalIsCorrelatedOnce(t *testing.T) {
	t.Parallel()
	var owner int
	for _, tree := range licensedTrees {
		walkHandWrittenGoFiles(t, tree.root, func(path, text string) {
			found := storedOriginalFallback.FindAllString(text, -1)
			if path == storedOriginalFile {
				owner = len(found)
				return
			}
			if len(found) > 0 {
				t.Errorf("%s correlates an activity to raw_capture itself (%v). Use "+
					"storekit.StoredOriginalSQL: a copy can drop the link column and stop "+
					"reaching every original whose key differs from its activity's", path, found)
			}
		})
	}
	if owner == 0 {
		t.Errorf("%s no longer spells the fallback arm, so this gate is matching nothing", storedOriginalFile)
	}
}
