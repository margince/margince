// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The audit trail is a SECOND door onto an activity's content, and every reader
// of it says what it does about a held one.
//
// audit_log.before / .after carry a row's columns verbatim — an activity's
// subject and body among them — reached through audit_log.entity_id rather than
// through the activity table. TestEveryReaderOfTheActivityTableExcludesRestrictedRows
// selects its subjects by "names the activity table", so no reader of this door
// has ever been one of them. Two of the three the issue found gate correctly;
// the verdict that census gave all three is the verdict it would give a file
// that gated nothing at all (#2138).
//
// This does NOT judge them. Whether a compliance trail should skip, blank or
// keep the image of a statutorily held activity is a decision with an argument
// on each side — audit_log is append-only and the hold is on the ACTIVITY, so
// blanking puts the trail out of step with what it recorded, while a hold every
// admin can read around through a second door is not much of a hold. That
// decision is #2138's open half and is not this gate's to make.
//
// What it does instead is make the door impossible to walk through quietly:
// each reader is named with what it does today, and a fourth one has to arrive
// here and say. The register is held by AssertAllMatched, so an entry whose
// file stops reading the trail cannot leave its answer behind for whatever is
// written there next.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// auditTrailReaders are the privacy module's readers of the audit trail — the
// files that serve it to somebody as a trail, rather than the seams elsewhere
// that read a row to repair or supersede one.
//
// Each entry says what the file does about a HELD activity, which is the
// question this door raises and the one the census next door cannot ask.
var auditTrailReaders = gatekit.Waive(map[string]string{
	"auditlog.go": "the compliance log. It withholds the IMAGE of an activity the reader may not see — UnscrubbedImageSQL, plus content_readable from the audience arm — while keeping the ROW, because a trail with holes in it is its own defect. A statutorily held activity is a DIFFERENT rule and is not withheld here: an admin can read its subject through this door. That is #2138's open half, deferred with its reasons rather than answered by whoever touched this file last",

	"fieldhistory.go": "the per-field history, projecting before/after to render one column's changes. Its window is bounded by scrubBoundary, so everything strictly older than a certified scrub is withheld — the erasure question. The retention-hold question rides the same open decision as the compliance log above",

	"edgehistory.go": "the history of the LINKS a record is an end of. It projects no image columns of its own: the shared column list carries them, and the rows it adds are edge rows whose images are the edge's, not the activity's",

	"edgeerasure.go": "not a reader served to anybody — it is the erasure path's own write-side probe, and UnscrubbedImageSQL appears here because it is what erasure asks about, not what it shows",
})

// trailReadPattern is the door: a SQL literal selecting from audit_log. Matched
// on the table rather than on before/after, because the projection and the FROM
// are routinely different Go string literals — recordAuditColumns lives in
// another file from the query that splices it — so a pattern wanting both in
// one literal finds one of these files and reports the rest clean.
var trailReadPattern = regexp.MustCompile(`(?is)\bFROM\s+audit_log\b`)

const trailReaderDir = "internal/modules/privacy"

func TestEveryReaderOfTheAuditTrailSaysWhatItDoesAboutAHeldActivity(t *testing.T) {
	t.Parallel()
	defer auditTrailReaders.AssertAllMatched(t)

	entries, err := os.ReadDir(trailReaderDir)
	if err != nil {
		t.Fatalf("reading %s: %v", trailReaderDir, err)
	}
	var found []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(filepath.Join(trailReaderDir, name))
		if err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if !trailReadPattern.Match(source) {
			continue
		}
		found = append(found, name)
		if auditTrailReaders.Waived(t, name) {
			continue
		}
		t.Errorf("%s reads the audit trail and says nothing about a held activity.\n"+
			"  audit_log.before/.after carry an activity's subject and body verbatim, reached through "+
			"entity_id rather than through the activity table — so the restricted-reader census next "+
			"door never sees this file, whatever it does.\n"+
			"  Name it in auditTrailReaders with what it does about a held row: withholds the image, "+
			"bounds its window, projects none, or defers to the open decision in #2138 with the reason.",
			name)
	}
	sort.Strings(found)

	// A census that recognised no reader would report the door closed in the
	// same words as a door nobody had built yet.
	if len(found) < 3 {
		t.Fatalf("found %d reader(s) of the audit trail under %s and expected at least 3 — this census "+
			"has stopped recognising the read rather than the tree having lost them: %v",
			len(found), trailReaderDir, found)
	}
	t.Logf("audit-trail readers: %v", found)
}
