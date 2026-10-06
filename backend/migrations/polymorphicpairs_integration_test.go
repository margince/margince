// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every polymorphic pair says what happens when the row it names goes away.
//
// A pair is one <noun>_id with no foreign key beside a <noun>_type or <noun>_kind that
// says which table the id is in. Postgres cannot hold a foreign key across that, so
// nothing stops the id outliving its row: a reader gets an empty result and no error,
// and an erased contact leaves rows behind in every table that names it this way.
//
// Three answers end a pair, and all three take it OUT of this corpus, which is why the
// corpus is derived rather than listed. SHAPE replaces the id with one nullable foreign
// key per branch — activity_link, dedupe_candidate and provider_run already did it, and
// none of them appears here. SPLIT gives each branch its own table, taking the
// discriminator with it. A pair that OUTLIVES its record on purpose stays, and owes a
// reason instead.
//
// What this holds meanwhile is that the set cannot grow unnoticed. It found
// list_live_member.entity_id, which arrived after the thirty-six were counted.

import (
	"context"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// polymorphicPairs finds every <noun>_id column with no foreign key whose table also
// carries the matching <noun>_type or <noun>_kind.
//
// Taken from the census that counted these rather than written again: the vocabulary
// obligation and this existence one police the same set, and one definition means a
// pair answers to both or to neither.
const polymorphicPairs = `
WITH cols AS (
  SELECT c.table_name, c.column_name FROM information_schema.columns c
    JOIN information_schema.tables t ON t.table_schema = c.table_schema
     AND t.table_name = c.table_name AND t.table_type = 'BASE TABLE'
   WHERE c.table_schema = 'public'),
fk AS (
  SELECT DISTINCT tc.table_name, kcu.column_name
    FROM information_schema.table_constraints tc
    JOIN information_schema.key_column_usage kcu
      ON kcu.constraint_name = tc.constraint_name AND kcu.table_schema = tc.table_schema
     -- Scoped to the table as well: a constraint name is unique per table, not per
     -- schema, so without this an FK on one table can pair with another table's key
     -- column and mark THAT column as keyed. The pair would then leave this corpus
     -- unnoticed, which is the one direction a census must not fail in. No two tables
     -- share a constraint name today; this is for the first pair that does.
     AND kcu.table_name = tc.table_name
   WHERE tc.table_schema = 'public' AND tc.constraint_type = 'FOREIGN KEY')
SELECT i.table_name || '.' || i.column_name || ' (' || d.column_name || ')'
  FROM (SELECT c.table_name, c.column_name, regexp_replace(c.column_name, '_id$', '') AS stem
          FROM cols c LEFT JOIN fk ON fk.table_name = c.table_name
                                  AND fk.column_name = c.column_name
         WHERE fk.column_name IS NULL AND c.column_name ~ '_id$') i
  JOIN cols d ON d.table_name = i.table_name
             AND d.column_name IN (i.stem || '_type', i.stem || '_kind')
 ORDER BY 1`

// TestEveryPolymorphicPairAnswersForItsRecord holds the register against the schema,
// in both directions: a pair nobody classified is a finding, and a line whose pair has
// been resolved fails until it goes.
func TestEveryPolymorphicPairAnswersForItsRecord(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	rows, err := conn.Query(ctx, polymorphicPairs)
	if err != nil {
		t.Fatalf("reading the polymorphic pairs: %v", err)
	}
	defer rows.Close()
	var found []string
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		found = append(found, one)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the polymorphic pairs: %v", err)
	}

	// NO COUNT FLOOR, and the reason is worth stating because the sibling gates in this
	// package carry one. Theirs count the whole corpus — every foreign key — which does
	// not shrink as the work lands. This corpus is the UNRESOLVED pairs, and resolving
	// one removes it from the catalog, so a floor here would fire on success: eight
	// pairs fixed and the gate would report that the census had stopped reading.
	//
	// The collapse it would have guarded is already covered. gatekit reports an entry
	// that stops matching, so a query returning nothing leaves all 37 register entries
	// stale and fails loudly — and that same report is the second direction, which is
	// what stops a pair that gains its ending from leaving its line behind.
	defer pairResolution.AssertAllMatched(t)
	for _, pair := range found {
		if pairResolution.Waived(t, pair) {
			continue
		}
		t.Errorf("%s names a record with nothing to say when that record goes.\n"+
			"Give it one of the three endings — a foreign key per branch (SHAPE), a table per "+
			"branch (SPLIT), or a reason it outlives the record (OUTLIVES) — and put it in "+
			"pairResolution with which and why.", pair)
	}
}

// pairResolution is what each polymorphic pair is going to do about the row it names,
// and why.
//
// Each entry is work owed, not a cost accepted, which is the one way this differs from
// the waivers it is spelled as: gatekit gives the reporting — a pair that gains its
// ending has to lose its line in the same change — and the entries are meant to go.
//
// Four endings, grouped by the analysis of which is cheaper per table: a shape costs one
// column per branch, a split costs a table of the width minus one, so the split wins
// only where the table is narrow and the vocabulary short.
//
// DECIDE is the honest one: seven pairs where what the reference MEANS is unsettled, so
// no ending can be picked. OUTLIVES is the one that stays, and its reason is the whole
// entry.
var pairResolution = gatekit.Waive(map[string]string{
	"activity_audience_member.subject_id (subject_type)":                 "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"ai_call.subject_id (subject_type)":                                  "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"ai_feedback.subject_id (subject_type)":                              "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"ai_task_run.subject_id (subject_type)":                              "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"analytics_share.scope_id (scope_kind)":                              "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"approval.co_target_entity_id (co_target_entity_type)":               "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"approval.target_entity_id (target_entity_type)":                     "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"assurance_exception.subject_id (subject_kind)":                      "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"assurance_task_item.subject_id (subject_kind)":                      "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"attachment.entity_id (entity_type)":                                 "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"audit_log.actor_id (actor_type)":                                    "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"audit_log.entity_id (entity_type)":                                  "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"communication_decision.subject_id (subject_kind)":                   "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"consent_qualifying_event.source_entity_id (source_entity_type)":     "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"contact_acquisition_evidence.source_entity_id (source_entity_type)": "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"deal_stage_evidence.source_id (source_type)":                        "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"embedding.entity_id (entity_type)":                                  "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"field_provenance.object_id (object_type)":                           "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"forecast_call.scope_id (scope_kind)":                                "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"forecast_snapshot.scope_id (scope_kind)":                            "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"list_live_member.entity_id (entity_type)":                           "SHAPE — the live projection of list_member, carrying the same five-value vocabulary, so it takes the shape of the table it projects: classified differently, a member would mean one thing in the source and another in the cache",
	"list_member.entity_id (entity_type)":                                "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"list_member_event.entity_id (entity_type)":                          "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"mail_draft.anchor_id (anchor_type)":                                 "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"notice.target_id (target_type)":                                     "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"record_grant.record_id (record_type)":                               "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"record_grant.subject_id (subject_type)":                             "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"report_edition_contribution.source_id (source_type)":                "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"sales_target.scope_id (scope_kind)":                                 "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"signal.entity_id (entity_type)":                                     "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"system_log.actor_id (actor_type)":                                   "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"taggable.entity_id (entity_type)":                                   "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"user_record_view.entity_id (entity_type)":                           "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"webhook_delivery.entity_id (entity_type)":                           "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"webhook_delivery.event_id (event_type)":                             "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"weekly_plan_commitment.linked_record_id (linked_record_type)":       "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"weekly_review_learning_citation.subject_id (subject_type)":          "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
})
