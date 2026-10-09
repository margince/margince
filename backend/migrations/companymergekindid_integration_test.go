// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The companion of the foreign-key census for the other way a row names a
// company: by an entity_type and entity_id pair, with no foreign key.
// The schema cannot see that pair. A table pointing at a company this way must
// be found in the catalog, or its rows stay on the archived record unnoticed.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// theKindIDFloor is the count below which the derivation is reading a smaller
// schema than it thinks: 14 such pairs exist today, 9 of which admit a company.
const theKindIDFloor = 8

// kindIDPairsTheMergeLeaves are the pairs the merge does not move,
// each with the reason moving it would be wrong.
var kindIDPairsTheMergeLeaves = gatekit.Waive(map[string]string{
	"approval.target_entity_id":                     "an approval pins the record version it was decided against; moving it would have it vouch for a version of the survivor it never saw",
	"approval.co_target_entity_id":                  "the co-target pins a record version exactly as the target does",
	"audit_log.entity_id":                           "the audit trail is the history of the retired record, which stays addressable through its merged_into pointer",
	"contact_acquisition_evidence.source_entity_id": "names the mail that evidences how a contact was acquired; its only writer records an activity, never a company",
	"embedding.entity_id":                           "a vector is derived from the retired record's own text, so moving it would give the survivor a vector of words it does not hold; the survivor is embedded from its own",
	"user_record_view.entity_id":                    "a reader's last-seen mark is written only by company360.RecordVisit, whose GREATEST rule is the whole of its correctness; a second writer here could rewind it, and a mark left on a retired record is read by nothing",
	"list_live_member.entity_id":                    "a live list's membership is recomputed from the list's definition on every evaluation, so a stale pair is replaced rather than carried",
	"list_member_event.entity_id":                   "the log of membership changes is history, and it is read for the record it was written against",
	"webhook_delivery.entity_id":                    "a delivery log is the history of what was sent about the retired record",
})

// TestEveryCompanyKindIDReferenceJoinsTheMerge derives the pairs from the
// catalog: an entity_id column beside an entity_type column.
// A closed vocabulary that excludes company drops the pair.
func TestEveryCompanyKindIDReferenceJoinsTheMerge(t *testing.T) {
	defer kindIDPairsTheMergeLeaves.AssertAllMatched(t)

	written := tablesTheMergeWrites(t)
	if len(written) == 0 {
		t.Fatal("the merge path's source yielded no written tables — the scan is broken, not the merge")
	}

	ownerDSN, _ := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)

	rows, err := owner.Query(context.Background(), `
		SELECT c.relname, a.attname
		FROM pg_attribute a
		JOIN pg_class c ON c.oid = a.attrelid AND c.relkind = 'r'
		JOIN pg_namespace n ON n.oid = c.relnamespace AND n.nspname = 'public'
		JOIN pg_attribute k ON k.attrelid = a.attrelid AND NOT k.attisdropped
		 AND k.attname = regexp_replace(a.attname, 'entity_id$', 'entity_type')
		WHERE a.attname ~ 'entity_id$' AND NOT a.attisdropped
		  AND NOT EXISTS (
		    SELECT 1 FROM pg_constraint ck
		    WHERE ck.conrelid = a.attrelid AND ck.contype = 'c'
		      AND k.attnum = ANY (ck.conkey)
		      AND pg_get_constraintdef(ck.oid) ~ 'ARRAY'
		      AND pg_get_constraintdef(ck.oid) !~ '''company''')
		ORDER BY 1, 2`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()

	seen := 0
	for rows.Next() {
		var table, column string
		if err := rows.Scan(&table, &column); err != nil {
			t.Fatal(err)
		}
		seen++
		key := table + "." + column
		if written[key] || kindIDPairsTheMergeLeaves.Waived(t, key) {
			continue
		}
		t.Errorf("%s can name a company by kind and id and the merge never moves it.\n"+
			"Left alone it names an archived record: nothing errors and the row is gone from the "+
			"survivor's view. Move it in internal/modules/contacts/mergekindid.go, or declare "+
			"it in kindIDPairsTheMergeLeaves with what moving it would cost.", key)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if seen < theKindIDFloor {
		t.Errorf("the census found %d kind-and-id pairs, fewer than the %d floor: it is reading a "+
			"smaller schema than it thinks", seen, theKindIDFloor)
	}
}
