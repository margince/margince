// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// The timeline reads the activities module's own projection, and adds exactly
// one column of its own.
//
// Three columns went missing from a hand-written copy of that list — the
// transport a message came over, the version an audience write needs, and the
// system a transcript was read from — each found by a test naming that one
// column, none of which could have found the next. The copy is gone; this is
// what stops one growing back.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/modules/activities"
)

func TestTheTimelineSelectsTheSharedProjectionAndOneColumnOfItsOwn(t *testing.T) {
	t.Parallel()
	const arm = "TRUE"
	shared := activities.ActivityProjection(arm)

	// The whole shared list, unaltered and first. Anything else — a subset, a
	// reordering, a second spelling of one of its columns — is the copy this
	// exists to prevent, and the extra below could then land on a destination
	// that is not its own.
	if !strings.HasPrefix(timelineProjection(arm), shared+",") {
		t.Fatalf("the timeline's select list does not open with the shared projection:\n%s",
			timelineProjection(arm))
	}

	// And the extra is ONE column, after it. The scan appends a single
	// destination to the shared ones, so a second extra would be read into the
	// first one's place with no error to say so.
	extra := strings.TrimPrefix(timelineProjection(arm), shared+",")
	if strings.Count(extra, " AS ") != 1 || !strings.Contains(extra, "AS filed_here") {
		t.Fatalf("the timeline adds %q — the scan takes exactly one extra, named filed_here", extra)
	}
}
