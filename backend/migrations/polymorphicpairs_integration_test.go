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
// Three answers end a pair, and all three take it OUT of this corpus. That is why the
// corpus is derived rather than listed. SHAPE gives each branch a foreign key column of
// its own. activity_link, dedupe_candidate and provider_run replaced the id with them.
// record_grant and others keep the id and derive the keys from it as stored generated
// columns. assurance_task_item has a trigger fill plain key columns instead,
// because a generated column would rewrite its history. A validated CHECK holds
// them to one key per row. SPLIT gives each branch its
// own table, taking the discriminator with it. A pair that OUTLIVES its record by
// design stays, and owes a reason instead.
//
// What this holds meanwhile is that the set cannot grow unnoticed. It found
// list_live_member.entity_id, which arrived after the thirty-six were counted.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

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
SELECT i.table_name, i.column_name, d.column_name
  FROM (SELECT c.table_name, c.column_name, regexp_replace(c.column_name, '_id$', '') AS stem
          FROM cols c LEFT JOIN fk ON fk.table_name = c.table_name
                                  AND fk.column_name = c.column_name
         WHERE fk.column_name IS NULL AND c.column_name ~ '_id$') i
  JOIN cols d ON d.table_name = i.table_name
             AND d.column_name IN (i.stem || '_type', i.stem || '_kind')
 ORDER BY 1, 2`

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
	var candidates []polymorphicPair
	for rows.Next() {
		var pair polymorphicPair
		if err := rows.Scan(&pair.table, &pair.id, &pair.discriminator); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		candidates = append(candidates, pair)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the polymorphic pairs: %v", err)
	}
	var found []string
	for _, pair := range candidates {
		if shapeAnswers(readShapeFacts(ctx, t, conn, pair)) {
			continue
		}
		found = append(found, pair.table+"."+pair.id+" ("+pair.discriminator+")")
	}

	// NO COUNT FLOOR, and the reason is worth stating because the sibling gates in this
	// package carry one. Theirs count the whole corpus — every foreign key — which does
	// not shrink as the work lands. This corpus is the UNRESOLVED pairs, and resolving
	// one removes it from the catalog, so a floor here would fire on success: eight
	// pairs fixed and the gate would report that the census had stopped reading.
	//
	// The collapse it would have guarded is already covered. gatekit reports an entry
	// that stops matching, so a query returning nothing leaves every register entry
	// stale. That same report stops a resolved pair from leaving its line behind.
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
	"deal_stage_evidence.source_id (source_type)":                        "DECIDE — evidence is refuted, never deleted, so a cascade would erase the reason for an earlier stage decision. RESTRICT (an activity or contract with evidence cannot be hard-deleted) or OUTLIVES (the pair outlives its source) is still to be chosen",
	"activity_audience_member.subject_id (subject_type)":                 "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"ai_call.subject_id (subject_type)":                                  "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"ai_feedback.subject_id (subject_type)":                              "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"ai_task_run.subject_id (subject_type)":                              "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"approval.co_target_entity_id (co_target_entity_type)":               "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"approval.target_entity_id (target_entity_type)":                     "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"assurance_exception.subject_id (subject_kind)":                      "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"attachment.entity_id (entity_type)":                                 "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"audit_log.actor_id (actor_type)":                                    "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"audit_log.entity_id (entity_type)":                                  "OUTLIVES — the row is a record of what happened and is meant to survive the record it names; the reference resolving to nothing is the designed end state, not a leak",
	"communication_decision.subject_id (subject_kind)":                   "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"consent_qualifying_event.source_entity_id (source_entity_type)":     "SPLIT — narrow table, two or three branches: one table per branch costs less than the columns a shape would add, and removes the discriminator with them",
	"contact_acquisition_evidence.source_entity_id (source_entity_type)": "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
	"embedding.entity_id (entity_type)":                                  "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"field_provenance.object_id (object_type)":                           "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"forecast_call.scope_id (scope_kind)":                                "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"forecast_snapshot.scope_id (scope_kind)":                            "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"list_live_member.entity_id (entity_type)":                           "SHAPE — the live projection of list_member, carrying the same five-value vocabulary, so it takes the shape of the table it projects: classified differently, a member would mean one thing in the source and another in the cache",
	"list_member.entity_id (entity_type)":                                "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"list_member_event.entity_id (entity_type)":                          "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"mail_draft.anchor_id (anchor_type)":                                 "SHAPE — one nullable foreign key per branch with ON DELETE CASCADE, and a CHECK binding the discriminator to exactly one, as activity_link carries",
	"notice.target_id (target_type)":                                     "DECIDE — what the reference means is not settled, so neither shape nor split can be chosen yet",
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

// readShapeFacts reads what shapeAnswers judges, off the catalog: the table's
// validated CHECKs, and the stored generated columns built from the pair's id.
func readShapeFacts(ctx context.Context, t *testing.T, conn *pgx.Conn, pair polymorphicPair) shapeFacts {
	t.Helper()
	facts := shapeFacts{pair: pair}
	checks, err := conn.Query(ctx, `
		SELECT pg_get_constraintdef(c.oid) FROM pg_constraint c
		 WHERE c.conrelid = $1::regclass AND c.contype = 'c' AND c.convalidated`, pair.table)
	if err != nil {
		t.Fatalf("reading the CHECKs of %s: %v", pair.table, err)
	}
	facts.checks, err = pgx.CollectRows(checks, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the CHECKs of %s: %v", pair.table, err)
	}
	keys, err := conn.Query(ctx, `
		SELECT gen.attname, pg_get_expr(ad.adbin, ad.adrelid),
		       EXISTS (SELECT 1 FROM pg_constraint fk WHERE fk.conrelid = gen.attrelid
		                  AND fk.contype = 'f' AND fk.conkey = ARRAY[gen.attnum])
		  FROM pg_attribute gen
		  JOIN pg_attrdef ad ON ad.adrelid = gen.attrelid AND ad.adnum = gen.attnum
		  JOIN pg_attribute src ON src.attrelid = gen.attrelid AND src.attname = $2
		  JOIN pg_depend dep ON dep.classid = 'pg_attrdef'::regclass AND dep.objid = ad.oid
		                    AND dep.refobjid = gen.attrelid AND dep.refobjsubid = src.attnum
		 WHERE gen.attrelid = $1::regclass AND gen.attgenerated = 's'`, pair.table, pair.id)
	if err != nil {
		t.Fatalf("reading the generated keys of %s: %v", pair.table, err)
	}
	facts.keys, err = pgx.CollectRows(keys, func(row pgx.CollectableRow) (branchKey, error) {
		var key branchKey
		err := row.Scan(&key.column, &key.expression, &key.keyed)
		return key, err
	})
	if err != nil {
		t.Fatalf("reading the generated keys of %s: %v", pair.table, err)
	}
	readTriggerFilledFacts(ctx, t, conn, &facts)
	return facts
}

// readTriggerFilledFacts reads what a trigger-filled key is judged by. That is
// each single-column foreign key, and the NOT NULL booleans a marker can be. It
// is also each `BEFORE` row trigger, with its function source.
func readTriggerFilledFacts(ctx context.Context, t *testing.T, conn *pgx.Conn, facts *shapeFacts) {
	t.Helper()
	table := facts.pair.table
	facts.foreignKeys = catalogRows(ctx, t, conn, table, `
		SELECT a.attname, fk.confrelid::regclass::text, fk.confdeltype = 'c', fk.convalidated
		  FROM pg_constraint fk
		  JOIN pg_attribute a ON a.attrelid = fk.conrelid AND fk.conkey = ARRAY[a.attnum]
		 WHERE fk.conrelid = $1::regclass AND fk.contype = 'f'`,
		func(row pgx.CollectableRow) (foreignKey, error) {
			var fk foreignKey
			err := row.Scan(&fk.column, &fk.references, &fk.cascades, &fk.validated)
			return fk, err
		})
	facts.flags = catalogRows(ctx, t, conn, table, `
		SELECT attname FROM pg_attribute
		 WHERE attrelid = $1::regclass AND attnum > 0 AND NOT attisdropped
		   AND attnotnull AND atttypid = 'boolean'::regtype`, pgx.RowTo[string])
	// tgtype bits: 1 row, 2 before, 4 insert, 16 update.
	facts.triggers = catalogRows(ctx, t, conn, table, `
		SELECT tg.tgtype & 4 <> 0, tg.tgtype & 16 <> 0, tg.tgqual IS NOT NULL,
		       cardinality(tg.tgattr::int2[]) > 0, tg.tgenabled::text, p.prosrc
		  FROM pg_trigger tg JOIN pg_proc p ON p.oid = tg.tgfoid
		 WHERE tg.tgrelid = $1::regclass AND NOT tg.tgisinternal AND tg.tgtype & 3 = 3`,
		func(row pgx.CollectableRow) (rowTrigger, error) {
			var trigger rowTrigger
			err := row.Scan(&trigger.onInsert, &trigger.onUpdate, &trigger.qualified,
				&trigger.columns, &trigger.enabled, &trigger.source)
			return trigger, err
		})
}

// catalogRows runs one catalog query about table and collects its rows.
func catalogRows[T any](
	ctx context.Context, t *testing.T, conn *pgx.Conn, table, query string, scan pgx.RowToFunc[T],
) []T {
	t.Helper()
	rows, err := conn.Query(ctx, query, table)
	if err != nil {
		t.Fatalf("reading the catalog of %s: %v", table, err)
	}
	got, err := pgx.CollectRows(rows, scan)
	if err != nil {
		t.Fatalf("reading the catalog of %s: %v", table, err)
	}
	return got
}
