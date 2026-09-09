// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every calendar pull lists OCCURRENCES, never recurring series masters.
//
// A cancellation is keyed on the event id the capture landed under
// (meetingmap.Settle → capture.CancelMeeting), so what one id refers to decides
// what one decline closes. Expanded, each occurrence carries its own id and
// declining next Tuesday closes next Tuesday's meeting. Unexpanded, a series
// master carries ONE id for every occurrence in the series, and a single
// decline would close the whole recurring meeting — every Monday standup for a
// year gone from the schedule off one refusal.
//
// It fails silently in the direction that matters. Dropping `singleEvents` from
// Google's query is a one-word edit that still compiles, still syncs, and still
// captures meetings; nothing about the resulting rows looks wrong until
// somebody declines one instance of a series. The connector tests cannot see it
// either — they decode fixtures, and a fixture is written to match whatever the
// query is assumed to return.
//
// So the QUERY is what this reads. Both connectors state the expansion in their
// own vendor's terms, which is why this asks for a per-connector marker rather
// than one shared string: Google passes singleEvents=true on every list, and
// Microsoft walks /me/calendarView, whose delta returns occurrences and not
// masters (the /me/events collection is the one that returns masters).

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// occurrenceExpansion names, per calendar connector, the file that issues its
// pull and the substring proving that pull is expanded.
//
// Keyed on the file that talks to the provider rather than on the package, so a
// second file taking over the pull fails here as a missing subject rather than
// passing unread.
var occurrenceExpansion = map[string]struct {
	// marker is the vendor's own expansion, as the query spells it.
	marker string
	// why says what the marker buys, for the reader who finds this red.
	why string
}{
	"internal/modules/capture/gcal/client.go": {
		marker: "singleEvents",
		why: "Google returns recurring series MASTERS unless singleEvents expands them, " +
			"and a master carries one id for the whole series",
	},
	"internal/modules/capture/graphcal/client.go": {
		marker: "calendarView",
		why: "Graph's calendarView returns occurrences; /me/events returns series masters, " +
			"which carry one id for the whole series",
	},
}

func TestEveryCalendarPullListsOccurrencesRatherThanSeriesMasters(t *testing.T) {
	t.Parallel()
	for where, want := range occurrenceExpansion {
		t.Run(where, func(t *testing.T) {
			t.Parallel()
			source, err := os.ReadFile(filepath.FromSlash(where))
			if err != nil {
				// The pull moved or the file was renamed. That is exactly the
				// edit this gate exists to notice: a census that cannot find
				// its subject has not passed, it has failed to look.
				t.Fatalf("reading %s: %v — if the pull moved, move its row in "+
					"occurrenceExpansion with it rather than deleting the row", where, err)
			}
			if !strings.Contains(string(source), want.marker) {
				t.Errorf("%s no longer names %q, so this calendar may be pulling recurring "+
					"series masters. %s — and a cancellation keys on the event id, so one "+
					"declined occurrence would close every occurrence in the series",
					where, want.marker, want.why)
			}
		})
	}
}
