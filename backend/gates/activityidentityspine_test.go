// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every door that mints an activity asks whether the record already has one.
//
// One real message is one row, whichever door it arrives through: an importer
// hands over a CRM history, a connector syncs the mailbox or calendar that held
// it, and both must land on the same activity. `activity_identity` is where the
// two doors agree, and the ONLY way a duplicate gets in is a mint site that
// never consults it.
//
// The defect this holds is silent in the direction that matters. A door that
// forgets the identity does not fail, does not warn, and does not look wrong
// locally — it simply writes a second row, and the duplicate surfaces days later
// on somebody's timeline with nothing to say where it came from. That is why
// this is a gate rather than a review note.
//
// The corpus is DERIVED: every non-test file that inserts into `activity` owes a
// reachable call to the identity resolve. A new door enrolls itself.
//
// What this CANNOT see, stated so nobody reads more into a pass than is here:
// it proves the call exists, not that the caller obeys the answer. A door that
// resolves an identity and then inserts anyway passes. The integration tests in
// internal/compose/importthencapture_integration_test.go are what cover the
// obeying, and they are mutation-checked against the resolve's removal.

import (
	"regexp"
	"sort"
	"strings"
	"testing"
)

// activityMintSite matches an INSERT into the `activity` table itself.
//
// The trailing [\s(] is the whole reason this does not also match
// `activity_link`, `activity_participant`, `activity_identity` and the six
// other child tables: Go's \b treats `_` as a word character, so without the
// class every one of them would enroll as a mint site and the gate would demand
// an identity claim from a table that has no identity.
var activityMintSite = regexp.MustCompile(`(?is)INSERT\s+INTO\s+activity[\s(]`)

// identityConsulted matches a call that asks activity_identity what this record
// already is — either door's spelling.
//
// The import door calls ResolveBindableIdentity directly. Capture cannot: it
// may not import a sibling module, so it holds the resolver as an injected seam
// and reaches it through activityHoldingIdentity. Both are named here because
// the question is "does this door ask", not "which function does it call".
var identityConsulted = regexp.MustCompile(
	`ResolveBindableIdentity|activityHoldingIdentity|boundToKnownMessage|recognizedMessage`)

func TestEveryActivityMintSiteConsultsTheSharedIdentity(t *testing.T) {
	t.Parallel()
	var sites []string
	for _, path := range goSourceFiles(t, ".") {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body := readGateFile(t, path)
		if !activityMintSite.MatchString(body) {
			continue
		}
		// A fixture seeder is not an ingestion door. It carries the `integration`
		// build tag, so it reaches no binary and no real message travels through
		// it — a seeded row is the test's own premise rather than a message
		// arriving twice.
		//
		// Recognised by the build tag rather than by path, because a path list is
		// the thing that goes stale: a seeder that moves keeps its tag.
		// isIntegrationTagged is the ownership gate's, shared rather than
		// re-spelled — one question, one answer.
		if isIntegrationTagged(path) {
			continue
		}
		sites = append(sites, path)
		if !identityConsulted.MatchString(body) {
			t.Errorf("%s inserts into `activity` and never consults the shared identity.\n\n"+
				"One message is one row whichever door it arrives through. A mint site that "+
				"does not ask activity_identity writes a second row for a message another "+
				"door already filed — silently, because nothing fails and the duplicate "+
				"surfaces later with nothing to say where it came from.\n\n"+
				"Ask activities.ResolveBindableIdentity before inserting, or reach it "+
				"through the injected seam if this package may not import activities.", path)
		}
	}
	sort.Strings(sites)
	// The census's own proof of life. Under-recognition is the one way this must
	// not break: a scan that stopped matching would report PASS over an empty
	// corpus and read exactly like a tree with no defect in it.
	if len(sites) < 2 {
		t.Fatalf("found %d activity mint site(s) — the scan for them has stopped working. "+
			"Both ingestion doors write this table (the import door in activities, the "+
			"capture door in capture), so fewer than two means the regex no longer matches "+
			"what it was written for, not that a door went away.\nfound: %s",
			len(sites), strings.Join(sites, ", "))
	}
}
