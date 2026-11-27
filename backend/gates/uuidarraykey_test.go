// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// A uuid array gives up the foreign key its ids would otherwise carry, so it
// says why.
//
// PostgreSQL has no per-element foreign key. When every id points at one table,
// a junction table keyed to it keeps the reference true, as site_read_proposal
// does. The census reads every uuid array column from the
// committed head catalog; each one names the reason it stays an array.

import (
	"bufio"
	"bytes"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// uuidArrayColumnFloor is below the count of uuid array columns the schema
// carries, so a catalog read that stopped seeing them fails.
const uuidArrayColumnFloor = 10

// catalogUUIDArrayColumn reads one uuid array column line.
var catalogUUIDArrayColumn = regexp.MustCompile(`^public\.([a-z_][a-z0-9_]*)\.([a-z_][a-z0-9_]*) uuid\[\]`)

// seatsAreRetired is why an array of app_user ids gains little from a key.
const seatsAreRetired = "app_user ids; a colleague is retired and never deleted, so a key would guard only the write"

// keylessUUIDArrays are the uuid arrays that keep no foreign key, each with why.
var keylessUUIDArrays = gatekit.Waive(map[string]string{
	"activity_commitment_admission.evidence_ids":        "the messages a recorded commitment decision read, published with it; a re-read replaces the decision whole, and a cascade would rewrite what it rested on",
	"activity_meeting_history.credited_seats_at_change": "a history row's snapshot of the seats a held meeting credited at that change; " + seatsAreRetired,
	"activity_participant.roster_seats":                 "the seats whose calendar rosters list the participant, folded whole by the address trigger and storekit.StoreRosterSeatsSQL; " + seatsAreRetired,
	"assurance_run.eligible_deal_owners":                "the owners a finished check run counted, frozen with the counts so its verdict reads as it was reached",
	"assurance_run.eligible_signal_owners":              "the owners a finished check run counted, frozen with the counts so its verdict reads as it was reached",
	"brief_item.evidence_ids":                           "the deal and then the activities a ranking cited, two tables in citation order; servedFindingSQL compares its length with what a reader still sees, so a cascade would hide a lost citation",
	"bulk_confirmation.affected":                        "contacts, companies, deals, leads or worklist items a bulk preview listed, five tables in one list, on a row that lapses after 15 minutes",
	"consent_qualifying_event.recorded_for_entities":    "the legal entities a consent ground was recorded for; consent evidence must outlive what it names",
	"mail_draft.attachment_ids":                         "a draft must never hold back the archive or erasure of a document, so each read resolves the ids and leaves out a gone one",
	"meeting_invitation.team_user_ids":                  "the colleagues whose time a team booking holds, written with the booking and found by a GIN index; " + seatsAreRetired,
	"site_read.proposal_ids":                            "site_read_proposal now holds the keyed copy; the array goes once core/1795716570_read_proposals_keyed has finished",
	"tag_interest_read.tag_ids":                         "a cached model answer read only under the vocabulary digest it was made with; tagsuggest.VocabularyDigest covers every tag id, so a removed tag retires the answer",
	"transcript_read.proposal_ids":                      "transcript_read_proposal now holds the keyed copy; the array goes once core/1795716570_read_proposals_keyed has finished",
})

func TestAUUIDArraySaysWhyItKeepsNoForeignKey(t *testing.T) {
	t.Parallel()
	raw, err := gatekit.ReadFile(vocabularyCatalog)
	if err != nil {
		t.Fatal(err)
	}
	var columns []string
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		if m := catalogUUIDArrayColumn.FindStringSubmatch(scanner.Text()); m != nil {
			columns = append(columns, m[1]+"."+m[2])
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	if len(columns) < uuidArrayColumnFloor {
		t.Fatalf("read %d uuid array columns from %s, fewer than the %d the schema carries: the catalog format moved",
			len(columns), vocabularyCatalog, uuidArrayColumnFloor)
	}
	for _, column := range columns {
		if keylessUUIDArrays.Waived(t, column) {
			continue
		}
		t.Errorf("%s is a uuid array, which no foreign key can hold. If its ids all name one table, move them to a junction table keyed to it; otherwise waive it here with the reason it stays an array",
			column)
	}
	keylessUUIDArrays.AssertAllMatched(t)
}
