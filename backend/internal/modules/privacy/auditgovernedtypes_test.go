// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The three lists that have to agree about which audit rows answer to an
// activity's audience.
//
// auditGovernedTypes decides which rows are JOINED and tested. The route SQL
// decides which of them can actually resolve an activity. The governance maps
// decide what survives redaction once a row is withheld. A type present in one
// and missing from another does not fail to compile and does not fail any
// existing test: it either withholds an image that had governance data in it,
// or — the direction that matters — leaves a content-carrying image readable to
// a reader outside the audience. Both are silent.

import (
	"strings"
	"testing"
)

func TestEveryGovernedTypeIsRoutedAndHasKeys(t *testing.T) {
	for _, entityType := range auditGovernedTypes {
		t.Run(entityType, func(t *testing.T) {
			// The route is a CASE over entity_type spelled as SQL literals, so
			// the check is that this type has an arm. Without one the CASE falls
			// to NULL, the activity never joins, and the row is withheld whole —
			// safe, but it destroys the governance record with no failing test.
			if !strings.Contains(auditActivityRouteSQL, "'"+entityType+"'") {
				t.Errorf("%q is governed but auditActivityRouteSQL has no arm for it, so its "+
					"activity never resolves and its image is withheld whole", entityType)
			}
			if len(governanceKeysFor(entityType)) == 0 {
				t.Errorf("%q is governed but has no governance keys, so a withheld image loses "+
					"every key including the ones recording who acted", entityType)
			}
		})
	}
}

func TestTheRouteCarriesNoTypeThatIsNotGoverned(t *testing.T) {
	// The other direction. A type routed but absent from auditGovernedTypes
	// resolves an activity that the content_readable predicate then never
	// consults — the join runs, the answer is discarded, and the image is
	// disclosed. That is the leak this PR closes, in the shape it would come
	// back.
	governed := make(map[string]bool, len(auditGovernedTypes))
	for _, entityType := range auditGovernedTypes {
		governed[entityType] = true
	}
	for _, line := range strings.Split(auditActivityRouteSQL, "\n") {
		_, rest, found := strings.Cut(line, "WHEN '")
		if !found {
			continue
		}
		entityType, _, ok := strings.Cut(rest, "'")
		if !ok {
			t.Fatalf("a WHEN arm with no closing quote — the route is malformed: %q", line)
		}
		if !governed[entityType] {
			t.Errorf("auditActivityRouteSQL routes %q but auditGovernedTypes omits it, so its "+
				"audience is resolved and then ignored", entityType)
		}
	}
}

func TestNoGovernanceKeySetAdmitsAKnownContentKey(t *testing.T) {
	// The keys every writer in this tree uses for the message's own words. None
	// may appear in any key set, on any type. `body` is the documented exception
	// on the activity: the writer reduces it to a presence flag and
	// isJSONBool re-checks that here rather than trusting it.
	// line_count and proposals are here because a MEASUREMENT of content is
	// content when the content is what the audience withholds: both say how
	// long the held conversation was.
	content := []string{
		"subject", "filename", "title", "source_id", "snippet", "text",
		"line_count", "proposals",
	}
	for _, entityType := range auditGovernedTypes {
		keys := governanceKeysFor(entityType)
		for _, key := range content {
			if _, admitted := keys[key]; admitted {
				t.Errorf("%q admits %q, which carries the message's own words to a reader "+
					"outside its audience", entityType, key)
			}
		}
	}
}
