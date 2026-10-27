// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every ON DELETE CASCADE foreign key can find its children by index.
//
// Postgres has to locate the child rows before it can cascade a delete, so a
// referencing column with no index makes that a sequential scan of the child
// table — inside the parent's delete transaction, holding its locks. Article 17
// erasure deletes contacts by design and cascades into five such tables at once.
//
// The set is DERIVED from pg_constraint rather than listed here. A gate that
// carried its own copy of the schema would pass over the next cascading key
// somebody adds, which is the only failure that matters: the indexes this
// asserts are cheap to create on an empty table and expensive on a full one.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/testdb"
)

func TestEveryCascadingKeyCanFindItsChildren(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	cascades, err := testdb.ForeignKeyCount(ctx, conn, "public", testdb.CascadingKeys)
	if err != nil {
		t.Fatal(err)
	}
	// The schema declared 232 when this landed. A count that collapses means the
	// query stopped recognising them, and every one would then read as covered.
	if cascades < 200 {
		t.Fatalf("this gate found %d cascading foreign key(s) and expects at least 200 — it has "+
			"stopped recognising them rather than the schema having given them up", cascades)
	}

	if uncovered := uncoveredIn(ctx, t, conn, testdb.CascadingKeys); len(uncovered) > 0 {
		t.Errorf("%d cascading foreign key(s) have no index their delete can use:\n\t%s\n"+
			"Deleting the parent scans the whole child table, inside the parent's transaction "+
			"and holding its locks. Add an index on the referencing columns in the constraint's "+
			"own order — it is instant on an empty table and a CONCURRENTLY with a maintenance "+
			"window once there is data.",
			len(uncovered), strings.Join(described(uncovered), "\n\t"))
	}
}

// TestTheCascadeIndexQueryNoticesAWithdrawnIndex takes an index the schema is
// relying on away again, inside a transaction it rolls back. An empty result is
// the one way the gate above can fail short — a `covered` clause that matches
// everything reads as a clean schema — so the discrimination is proven rather
// than assumed.
func TestTheCascadeIndexQueryNoticesAWithdrawnIndex(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil {
			t.Errorf("rollback: %v", err)
		}
	}()

	const anIndexACascadeUses = `
SELECT i.indexrelid::regclass::text, fk.child || '.' || fk.conname
  FROM (SELECT c.conrelid AS childoid, c.conrelid::regclass::text AS child, c.conkey, c.conname,
               (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
                  FROM unnest(c.conkey) WITH ORDINALITY AS u(k, ord)
                  JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = u.k) AS cols
          FROM pg_constraint c
          JOIN pg_class t ON t.oid = c.conrelid
          JOIN pg_namespace n ON n.oid = t.relnamespace
         WHERE c.contype = 'f' AND c.confdeltype = 'c' AND n.nspname = 'public') fk
  JOIN pg_index i ON i.indrelid = fk.childoid
 WHERE i.indisvalid AND i.indpred IS NULL AND NOT i.indisprimary
   AND (SELECT array_agg(k ORDER BY ord) FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord))
       = (SELECT array_agg(a ORDER BY ord)
            FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS x(a, ord)
           WHERE ord <= array_length(fk.conkey, 1))
 ORDER BY 1 LIMIT 1`

	var index, key string
	if err := tx.QueryRow(ctx, anIndexACascadeUses).Scan(&index, &key); err != nil {
		t.Fatalf("finding an index a cascade uses: %v", err)
	}
	if _, err := tx.Exec(ctx, "DROP INDEX "+pgx.Identifier{index}.Sanitize()); err != nil {
		t.Fatalf("dropping %s: %v", index, err)
	}

	uncovered := uncoveredIn(ctx, t, tx, testdb.CascadingKeys)
	if !slices.ContainsFunc(uncovered, func(u testdb.UncoveredKey) bool { return u.Constraint == key }) {
		t.Errorf("dropped %s, the only index serving the cascade on %s, and the query still "+
			"reported it covered — it is matching something other than a usable index, so a "+
			"schema with no index at all would read as clean", index, key)
	}
}

// uncoveredIn reads the public schema's uncovered keys of one action, through a
// connection or a transaction holding a withdrawn index.
func uncoveredIn(ctx context.Context, t *testing.T, q testdb.Querier, action string) []testdb.UncoveredKey {
	t.Helper()
	keys, err := testdb.UncoveredForeignKeys(ctx, q, "public", action)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func described(keys []testdb.UncoveredKey) []string {
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		out = append(out, key.String())
	}
	return out
}

// TestEveryOtherForeignKeyIsIndexedOrMeasured holds the keys the cascade gate
// above never looks at. A non-cascading parent delete still has to find the
// children: `SET NULL` then updates them, and `RESTRICT` and `NO ACTION` refuse on them.
// So each such key is indexed, or a measurement says the lookup is affordable.
// The measurement is of its parent in foreignKeyNoIndexNeeded, or of the key itself
// in foreignKeyScanAccepted.
func TestEveryOtherForeignKeyIsIndexedOrMeasured(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	others, err := testdb.ForeignKeyCount(ctx, conn, "public", testdb.OtherKeys)
	if err != nil {
		t.Fatal(err)
	}
	// The schema declared 218 when this landed. A collapse means the query stopped
	// recognising them, and every one would then read as covered.
	if others < 180 {
		t.Fatalf("this gate found %d non-cascading foreign key(s) and expects at least 180 — it "+
			"has stopped recognising them rather than the schema having given them up", others)
	}

	uncovered := uncoveredIn(ctx, t, conn, testdb.OtherKeys)
	for _, key := range uncovered {
		_, parentMeasured := foreignKeyNoIndexNeeded[key.Parent]
		_, keyMeasured := foreignKeyScanAccepted[key.Constraint]
		switch {
		case parentMeasured && keyMeasured:
			t.Errorf("%s is in foreignKeyScanAccepted and under a parent foreignKeyNoIndexNeeded "+
				"already measured: keep one reason, so a reader knows which one holds.", key)
		case !parentMeasured && !keyMeasured:
			t.Errorf("%s has no index its parent's delete can use, and no measurement accounts "+
				"for it.\nIndex the referencing columns, or measure the parent delete on seeded "+
				"data and record why the lookup is affordable in foreignKeyScanAccepted.", key)
		}
	}
	for key := range foreignKeyScanAccepted {
		if !slices.ContainsFunc(uncovered, func(u testdb.UncoveredKey) bool { return u.Constraint == key }) {
			t.Errorf("foreignKeyScanAccepted measures %s, which is indexed now or no longer "+
				"exists: delete the entry, so what remains is a judgement about the schema as it is", key)
		}
	}
	// A parent whose keys all gained an index, or which no longer has an
	// uncovered key at all, is a measurement with nothing left to answer for.
	for parent := range foreignKeyNoIndexNeeded {
		if !slices.ContainsFunc(uncovered, func(u testdb.UncoveredKey) bool { return u.Parent == parent }) {
			t.Errorf("foreignKeyNoIndexNeeded measures %q, which has no uncovered key left: "+
				"delete the entry, so what remains is a judgement about the schema as it is", parent)
		}
	}
}

// foreignKeyNoIndexNeeded records the parents measured as never deleted. Every
// key under one of them is accounted for, as its lookup never runs.
//
// gatekit:fixture the parent table each measurement was taken of, and what the
// measurement found. Not a waiver: its keys are parents, where a finding names one
// child key. The test above checks the directions, not subject matching.
var foreignKeyNoIndexNeeded = map[string]string{
	"deal_room_participant":      "a seat is revoked and anonymized in place by the erasure, never deleted: a comment names its participant row, and deleting the seat would orphan the conversation",
	"report_definition_revision": "append-only: a revision is what a report meant when it ran",
	"sdr_handoff_reason":         "a reason is a vocabulary row the handoffs that cite it keep resolving",
	"activity_kind":              "the kind vocabulary. Nothing deletes a kind, and an index on it would cost every insert of the busiest table in the schema to serve a delete that does not happen",
	"ai_call_config":             "a configuration is superseded rather than removed, so a call can still say what it ran under",
	"channel_provider":           "a provider is a vocabulary row; the identities bound to it keep naming it",
	"deal_acquisition_source":    "a source is vocabulary the deals that cite it keep resolving",
	"offer":                      "an offer is archived with its deal rather than deleted",
	"deal_room_document":         "a document is withdrawn in place, keeping the engagement rows that reference it readable",
	"maskable_field":             "the field vocabulary a mask policy cites",
	"meeting_invitation":         "append-only: an invitation is the record that it was sent",
	"oauth_client":               "a client is disabled in place so its grants stay attributable",
	"project_health_assessment":  "append-only: an assessment is what somebody judged at a point in time",
	"company":                    "a company is archived, never deleted: archived_at retires it and every read filters on it",
	"consent_purpose":            "a purpose is archived rather than deleted, so a policy that cited it keeps resolving",
	"activity":                   "an activity is archived, and the retention ladder redacts its content in place rather than removing the row",
	"passport":                   "a passport is revoked or left to expire; the row stays so an audit trail can still name what acted",
	"contact":                    "a contact is anonymized in place by the Art. 17 cascade, which is why that cascade exists rather than a delete",
	"team":                       "a team is archived, keeping the membership history that references it readable",
	"site_read":                  "append-only: a site read is evidence of what a scan saw, and nothing retires it",
	"project":                    "a project is archived or ended, both in place",
	"pipeline":                   "a pipeline is archived, so the deals that moved through it keep their stage history",
	"stage":                      "a stage is archived; deleting one would strip the history of every deal that passed it",
	"approval":                   "an approval is consumed or expires, and the row is the record that it was asked for",
	"contract":                   "a contract is archived, which is what a commercial record needs rather than removal",
	"deal":                       "a deal is archived or closed, never deleted",
	"forecast_call":              "append-only: a call is what somebody committed to at a point in time",
	"oauth_grant":                "a grant is revoked in place, so a later audit can say what the client once held",
	"tag":                        "a tag is archived, keeping the rows that carried it explainable",
	"commission_entry":           "append-only: a commission entry is a financial record",
	"communication_instruction":  "an instruction is consumed or revoked in place",
	"communication_review":       "append-only: a review is the decision somebody made",
	"consent_text_version":       "append-only, and the migration revokes DELETE on it: a proof row pointing at a removed version is the failure it prevents",
	"audit_log":                  "append-only by trigger (audit_log_immutable refuses DELETE), which is the whole point of the table",
	"close_date_run":             "append-only: a sweep run holds the cursor the next one resumes from",
	"deal_stage_evidence":        "append-only: evidence for why a stage moved",
	"stage_exit_criterion":       "a criterion is archived, so the evidence that met it keeps resolving",
	"deal_stage_history":         "append-only: the history is the record",
	"finance_connection":         "a connection is archived, keeping the entries it produced attributable",
	"product":                    "a product is archived; the offers that priced it still name it",
	"relationship":               "a relationship is archived or ended in place",
	"record_role":                "append-only: a role assignment is who could do what, when",
	"report_schedule":            "append-only in practice: a schedule is retired by its own flags rather than removed",
	"agent_run":                  "append-only: the run is how an outcome was reached",
	"comms_outbound":             "append-only: the ledger of what was sent",
	"weekly_review":              "append-only: a review is what a reader saw that week",
	"app_user": "nothing deletes an app_user row. A member is retired by setting " +
		"archived_at, live-member reads filter on it, and preservedResetTables keeps the table " +
		"out of the installation reset (compose/datasweep.go); the only DELETE FROM " +
		"app_user in the tree is in integration tests, over a handful of rows. " +
		"So the referential scan these 63 keys would need an index for never runs, and " +
		"adding one to each would cost every insert on 63 children to serve a delete " +
		"that does not happen. Should a hard delete arrive, this entry is what has to be revisited.",
}

// foreignKeyScanAccepted records single keys, by child.conname, whose parent is
// deleted and whose lookup a measurement found affordable without an index of its
// own. Each entry says what EXPLAIN ANALYZE of the real delete on seeded data found.
//
// gatekit:fixture each constraint and what its measurement found. Not a waiver: the
// directions are checked by the test above, not by subject matching.
var foreignKeyScanAccepted = map[string]string{}
