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
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
)

// The two halves of the schema's foreign keys, by what a parent delete does to the
// child. Spelled as clauses because the covered-ness query above is one statement and
// only its corpus differs: a cascade DELETES the child and must never scan for it,
// while the rest only have to FIND it — SET NULL then updates it, RESTRICT and NO
// ACTION refuse on it.
const (
	cascadeAction = `c.confdeltype = 'c'`
	otherActions  = `c.confdeltype <> 'c'`
)

// uncoveredKeysTemplate finds foreign keys, of whichever action the caller asks for,
// whose referencing columns are
// not the LEADING columns of some usable index on the child table. Leading, not
// merely present: an index on (b, a) does not serve a key on (a) alone — Postgres
// will happily scan that index end to end instead, which is the sequential scan
// again with more pages.
//
// Leading as a SET, though, not in the constraint's order. The referential probe
// equality-constrains every one of the key's columns at once, so a btree on (b, a)
// seeks just as directly for a key on (a, b); order would only matter for a column
// the probe left unconstrained, and it leaves none. No key in the schema is covered
// this way today — the comparison is a set because the rule is, not to admit one.
//
// Usable excludes an invalid index, which is never read, and a partial one,
// whose predicate the cascade's `col = $1` does not imply — `WHERE archived_at
// IS NULL` leaves out the rows an erasure still has to delete. `col IS NOT NULL`
// is the one predicate equality does imply, so those indexes count.
const uncoveredKeysTemplate = `
WITH fk AS (
  SELECT c.oid, c.conrelid AS childoid, c.conrelid::regclass::text AS child,
         c.confrelid::regclass::text AS parent, c.conkey
    FROM pg_constraint c
    JOIN pg_class t ON t.oid = c.conrelid
    JOIN pg_namespace n ON n.oid = t.relnamespace
   WHERE c.contype = 'f' AND n.nspname = 'public' AND %s
), covered AS (
  SELECT DISTINCT fk.oid
    FROM fk
    JOIN pg_index i ON i.indrelid = fk.childoid
   WHERE i.indisvalid
     -- indkey carries the INCLUDE columns too, and those are payload: an index on
     -- (b) INCLUDE (a) stores a but cannot seek on it. No index in the schema has
     -- any today, so this guard is here for the first one that does.
     AND i.indnkeyatts >= array_length(fk.conkey, 1)
     AND (i.indpred IS NULL
          OR (array_length(fk.conkey, 1) = 1
              AND pg_get_expr(i.indpred, i.indrelid)
                  = '(' || (SELECT att.attname FROM pg_attribute att
                             WHERE att.attrelid = fk.childoid AND att.attnum = fk.conkey[1])
                    || ' IS NOT NULL)'))
     AND (SELECT array_agg(k ORDER BY k) FROM unnest(fk.conkey) AS u(k))
         = (SELECT array_agg(a ORDER BY a)
              FROM unnest(i.indkey::int2[]) WITH ORDINALITY AS x(a, ord)
             WHERE ord <= array_length(fk.conkey, 1))
)
SELECT fk.child || ' (' || (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
          FROM unnest(fk.conkey) WITH ORDINALITY AS u(k, ord)
          JOIN pg_attribute att ON att.attrelid = fk.childoid AND att.attnum = u.k) || ')',
       fk.parent
  FROM fk
 WHERE fk.oid NOT IN (SELECT oid FROM covered)
 ORDER BY 1`

// keyCountTemplate tallies the keys of one action, indexed or not. It is the
// floor that stops this passing by reading nothing.
const keyCountTemplate = `
SELECT count(*) FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = 'public' AND %s`

func TestEveryCascadingKeyCanFindItsChildren(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var cascades int
	if err := conn.QueryRow(ctx, fmt.Sprintf(keyCountTemplate, cascadeAction)).Scan(&cascades); err != nil {
		t.Fatalf("counting cascading keys: %v", err)
	}
	// The schema declared 232 when this landed. A count that collapses means the
	// query stopped recognising them, and every one would then read as covered.
	if cascades < 200 {
		t.Fatalf("this gate found %d cascading foreign key(s) and expects at least 200 — it has "+
			"stopped recognising them rather than the schema having given them up", cascades)
	}

	if uncovered, _ := readUncovered(ctx, t, conn, cascadeAction); len(uncovered) > 0 {
		t.Errorf("%d cascading foreign key(s) have no index their delete can use:\n\t%s\n"+
			"Deleting the parent scans the whole child table, inside the parent's transaction "+
			"and holding its locks. Add an index on the referencing columns in the constraint's "+
			"own order — it is instant on an empty table and a CONCURRENTLY with a maintenance "+
			"window once there is data.",
			len(uncovered), strings.Join(uncovered, "\n\t"))
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
SELECT i.indexrelid::regclass::text, fk.child || ' (' || fk.cols || ')'
  FROM (SELECT c.conrelid AS childoid, c.conrelid::regclass::text AS child, c.conkey,
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

	uncovered, _ := readUncovered(ctx, t, tx, cascadeAction)
	if !slices.Contains(uncovered, key) {
		t.Errorf("dropped %s, the only index serving the cascade on %s, and the query still "+
			"reported it covered — it is matching something other than a usable index, so a "+
			"schema with no index at all would read as clean", index, key)
	}
}

// readUncovered runs uncoveredKeysTemplate against whatever the caller has — the
// connection, or a transaction holding a withdrawn index.
func readUncovered(ctx context.Context, t *testing.T, q interface {
	Query(context.Context, string, ...any) (pgx.Rows, error)
}, action string,
) ([]string, map[string]string) {
	t.Helper()
	rows, err := q.Query(ctx, fmt.Sprintf(uncoveredKeysTemplate, action))
	if err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	defer rows.Close()
	var uncovered []string
	parentOf := map[string]string{}
	for rows.Next() {
		var one, parent string
		if err := rows.Scan(&one, &parent); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		uncovered = append(uncovered, one)
		parentOf[one] = parent
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	return uncovered, parentOf
}

// foreignKeyIndexDebt is every non-cascading foreign key whose child carries no index
// the parent's delete can use. It is a REGISTER, not a waiver list: each line is work
// owed, and the test below fails in both directions — a key missing from it is one
// somebody added uncovered, and a line with no finding behind it is one somebody fixed
// and did not delete.
//
// It is not a judgement that indexing all 166 is right. Some are lookup parents nothing
// ever deletes (activity (kind) references a small kind table), where an index on a
// low-cardinality column costs every insert and buys nothing; others are SET NULL on
// hot children, where the scan is followed by an update of every row it found. Which
// ones to index is a measurement, tracked separately — what this register buys today is
// that the number cannot grow without somebody saying so.
// TestEveryOtherForeignKeyIsIndexedOrRegisteredDebt holds the 219 keys the cascade
// gate above never looks at.
//
// A non-cascading parent delete still has to FIND the children before it can decide:
// SET NULL scans the child and then updates every row it found, and RESTRICT and NO
// ACTION scan it to refuse. Deleting one app_user reaches 63 such tables, audit_log and
// system_log among them.
//
// The assertion is equality with the register rather than emptiness, because 166 of
// them are uncovered today and pretending otherwise would mean either a red gate
// nobody can merge past or no gate at all.
func TestEveryOtherForeignKeyIsIndexedOrRegisteredDebt(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var others int
	if err := conn.QueryRow(ctx, fmt.Sprintf(keyCountTemplate, otherActions)).Scan(&others); err != nil {
		t.Fatalf("counting non-cascading keys: %v", err)
	}
	// The schema declared 218 when this landed. A collapse means the query stopped
	// recognising them, and every one would then read as covered.
	if others < 180 {
		t.Fatalf("this gate found %d non-cascading foreign key(s) and expects at least 180 — it "+
			"has stopped recognising them rather than the schema having given them up", others)
	}

	uncovered, parentOf := readUncovered(ctx, t, conn, otherActions)
	registered := slices.Clone(foreignKeyIndexDebt)
	slices.Sort(registered)
	judged := map[string]bool{}
	for _, key := range uncovered {
		if _, measured := foreignKeyNoIndexNeeded[parentOf[key]]; measured {
			judged[key] = true
			continue
		}
		if !slices.Contains(registered, key) {
			t.Errorf("%s has no index its parent's delete can use, and neither the register nor "+
				"foreignKeyNoIndexNeeded accounts for it.\n"+
				"Index the referencing columns in the constraint's own order, add the line and say "+
				"in the PR why the scan is affordable there, or measure the parent and record what "+
				"you found.", key)
		}
	}
	for _, key := range registered {
		if !slices.Contains(uncovered, key) {
			t.Errorf("%s is in the register but is indexed now: delete its line, so the register "+
				"keeps meaning what it says.", key)
		}
		if judged[key] {
			t.Errorf("%s is in the register and under a parent foreignKeyNoIndexNeeded has "+
				"already measured: delete the line, or the register counts work nobody owes.", key)
		}
	}
	// A parent whose keys all gained an index, or which no longer has an
	// uncovered key at all, is a measurement with nothing left to answer for.
	// Left in place it reads as a standing judgement about a shape that has
	// moved on.
	for parent := range foreignKeyNoIndexNeeded {
		var still bool
		for _, key := range uncovered {
			if parentOf[key] == parent {
				still = true
				break
			}
		}
		if !still {
			t.Errorf("foreignKeyNoIndexNeeded measures %q, which has no uncovered key left: "+
				"delete the entry, so what remains is a judgement about the schema as it is", parent)
		}
	}
}

// foreignKeyNoIndexNeeded records the parents whose keys have been MEASURED and
// need no index, with what the measurement found. A key under one of these
// parents is accounted for without being work owed.
//
// The distinction matters because the register is a list of work: a key that
// will never want an index would otherwise sit in it forever, and a reader
// cannot tell those from the ones nobody has looked at yet.
//
// gatekit:fixture the parent table each measurement was taken of, and what the
// measurement found. Not a waiver: its keys are parents, where a finding names
// one child key, and the three directions below are checked here rather than by
// subject matching.
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
		"archived_at, every read filters on it, and preservedResetTables keeps the table " +
		"out of the installation reset (compose/datasweep.go); the only DELETE FROM " +
		"app_user in the tree is in three integration tests, over a handful of rows. " +
		"So the referential scan these 63 keys would need an index for never runs, and " +
		"adding one to each would cost every insert on 63 children to serve a delete " +
		"that does not happen. Migration 1790813926 removed fifteen indexes for exactly " +
		"that trade. Should a hard delete arrive, this entry is what has to be revisited, " +
		"and the keys return to the register below.",
}

var foreignKeyIndexDebt = []string{
	"attachment (supersedes_id)",
	"comms_outbound (link_id)",
	"communication_suppression (carried_from)",
	"contact_phone (superseded_phone_id)",
	"deal_room_document (attachment_id)",
	"deal_room_thread (attachment_id)",
	"preference_token (contact_email_id)",
	"voice_build (voice_profile_id, result_version)",
	"voice_learning_signal (voice_profile_id, profile_version)",
	"voice_profile_delta (voice_profile_id, from_version)",
	"voice_profile_version (voice_profile_id, predecessor_version)",
}
