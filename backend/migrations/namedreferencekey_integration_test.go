// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A uuid column named after a table references it, or says why it does not.
//
// A column read to find a live row and carrying no key returns nothing once
// its row is gone, and the caller cannot tell that from an empty answer. A
// column that must outlive its row (a trace, a frozen snapshot, an erasure
// record) is right to carry no key, and the waiver below holds its reason.
//
// The corpus is read from pg_attribute. A column names a table when it is
// `<table>_id`, or `<prefix>_<table>_id`, or a person column that names a
// seat. A sibling `<x>_type` or `<x>_kind` column makes the reference
// polymorphic, and no key can span several tables.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// uuidIDColumnFloor is below the count of uuid `_id` columns this schema
// carries. A reading under it means the census stopped seeing the schema.
const uuidIDColumnFloor = 600

// unkeyedReferences are the columns whose missing key is the behaviour.
var unkeyedReferences = gatekit.Waive(map[string]string{
	"assurance_deal_receipt.deal_id":            "the receipt is read only joined from a live deal, and it keeps no personal data a key would have to cascade",
	"assurance_run.subject_deal_id":             "a run records one past check, and readers only ask whether a subject was named",
	"capture_test_mailbox_sent.user_id":         "test mailbox bookkeeping that exists only behind a flag and carries no keys by design",
	"capture_trace.activity_id":                 "a trace is not a record: it outlives the activity, and its reads left join a row that may be erased",
	"capture_trace.user_id":                     "a trace is not a record, and the seat is a scope filter that never fetches the user",
	"chat_post_item.activity_id":                "erasure deletes activities, and the flush withdraws a post whose activity is gone; a key would refuse the erasure or lose the withdrawal",
	"consent_qualifying_event.about_deal_id":    "the record of what was asked must not go with a removed deal",
	"deal_stage_history.approval_id":            "a history row that is written and returned and never joined",
	"deal_stage_history.import_run_id":          "a provenance stamp that is never joined; the import tables are created after the history table",
	"erasure_suppression.lift_contact_id":       "an erasure record is kept, never deleted, and a check forbids clearing the lift",
	"forecast_contribution.approval_id":         "a frozen snapshot names what moved the deal when the forecast was taken",
	"forecast_contribution.deal_id":             "a frozen snapshot keeps one deal's part even after the deal changes",
	"forecast_contribution.owner_id":            "a frozen snapshot names who owned the deal when the forecast was taken",
	"forecast_contribution.stage_id":            "a stage deleted later must not take forecast history with it",
	"list_revision.team_id":                     "a revision is version history, compared only with the revision before it",
	"partner.activation_deal_id":                "evidence of the activation; a check requires it once a partner is active",
	"partner.activation_lead_id":                "evidence of the activation; erasure deletes leads, and a cleared value would break the pairing check",
	"project_phase_history.audit_log_id":        "the audit log is append only, and a history row must not block an erasure",
	"sales_initiative_baseline.stage_id":        "the baseline is written once at launch and never rewritten",
	"scheduled_send.agent_passport_id":          "provenance of the agent that scheduled the send; a passport is revoked, never deleted",
	"system_log.passport_id":                    "an operational log row outlives what it describes and is never joined",
	"team_weekly_review.team_id":                "a disbanded team must leave its past weeks readable",
	"team_weekly_review_member.user_id":         "the frozen roster of that week",
	"team_weekly_review_rep.user_id":            "the frozen membership of that week",
	"team_weekly_review_rep_funnel.pipeline_id": "names in a week are frozen, so a deleted pipeline leaves the week readable",
	"team_weekly_review_rep_funnel.stage_id":    "names in a week are frozen, so a deleted stage leaves the week readable",
	"weekly_review_deal.deal_id":                "the week keeps the deals it counted even after they change or go",
	"weekly_review_driver.deal_id":              "the week keeps the deals that drove it even after they change or go",
})

// namedReferenceSQL answers every uuid column that names a table and is
// covered by no foreign key, skipping a reference made polymorphic by a
// sibling type column.
const namedReferenceSQL = `
WITH cols AS (
  SELECT c.oid, c.relname AS tbl, a.attname AS col, a.attnum
    FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
   WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p') AND NOT c.relispartition
     AND a.attnum > 0 AND NOT a.attisdropped AND a.atttypid = 'uuid'::regtype
), named AS (
  SELECT cols.*, CASE
     WHEN col ~ '_id$' AND to_regclass('public.' || regexp_replace(col, '_id$', '')) IS NOT NULL THEN true
     WHEN col IN ('owner_id', 'user_id', 'assignee_id', 'decided_by', 'requested_by', 'created_by',
                  'changed_by', 'resolved_by', 'revoked_by', 'approved_by', 'captured_by') OR col ~ '_user_id$' THEN true
     WHEN col ~ '_id$' AND to_regclass('public.' || regexp_replace(regexp_replace(col, '_id$', ''), '^.*_', '')) IS NOT NULL THEN true
     ELSE false END AS names_table
    FROM cols
)
SELECT tbl || '.' || col
  FROM named
 WHERE names_table
   AND NOT EXISTS (SELECT 1 FROM pg_constraint k
                    WHERE k.conrelid = named.oid AND k.contype = 'f' AND named.attnum = ANY (k.conkey))
   AND NOT EXISTS (SELECT 1 FROM pg_attribute s
                    WHERE s.attrelid = named.oid AND NOT s.attisdropped
                      AND s.attname IN (regexp_replace(col, '_id$', '_type'), regexp_replace(col, '_id$', '_kind'),
                                        'entity_type', 'object_type', 'subject_type', 'subject_kind',
                                        'record_type', 'entity_kind'))
 ORDER BY 1`

func TestAColumnNamingATableReferencesItOrSaysWhyNot(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)

	ctx := context.Background()
	var corpus int
	if err := conn.QueryRow(ctx, `
		SELECT count(*) FROM pg_attribute a JOIN pg_class c ON c.oid = a.attrelid
		 WHERE c.relnamespace = 'public'::regnamespace AND c.relkind IN ('r', 'p') AND NOT c.relispartition
		   AND a.attnum > 0 AND NOT a.attisdropped AND a.atttypid = 'uuid'::regtype AND a.attname ~ '_id$'`).Scan(&corpus); err != nil {
		t.Fatalf("counting the uuid _id columns: %v", err)
	}
	if corpus < uuidIDColumnFloor {
		t.Fatalf("counted %d uuid _id columns, fewer than the %d this schema carries: the census is reading a smaller schema than it thinks",
			corpus, uuidIDColumnFloor)
	}
	rows, err := conn.Query(ctx, namedReferenceSQL)
	if err != nil {
		t.Fatalf("reading the named references: %v", err)
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("reading the named references: %v", err)
	}
	for _, column := range found {
		if unkeyedReferences.Waived(t, column) {
			continue
		}
		t.Errorf("%s names a table and carries no foreign key. Add the key, or waive it here with the reason the row must outlive what it names",
			column)
	}
	t.Logf("read %d uuid _id columns, %d of them unkeyed references", corpus, len(found))
	unkeyedReferences.AssertAllMatched(t)
}
