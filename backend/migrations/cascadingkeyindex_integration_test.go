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
  SELECT c.oid, c.conrelid AS childoid, c.conrelid::regclass::text AS child, c.conkey
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
          JOIN pg_attribute att ON att.attrelid = fk.childoid AND att.attnum = u.k) || ')'
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

	if uncovered := readUncovered(ctx, t, conn, cascadeAction); len(uncovered) > 0 {
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

	uncovered := readUncovered(ctx, t, tx, cascadeAction)
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
) []string {
	t.Helper()
	rows, err := q.Query(ctx, fmt.Sprintf(uncoveredKeysTemplate, action))
	if err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	defer rows.Close()
	var uncovered []string
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		uncovered = append(uncovered, one)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading uncovered cascades: %v", err)
	}
	return uncovered
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

	uncovered := readUncovered(ctx, t, conn, otherActions)
	registered := slices.Clone(foreignKeyIndexDebt)
	slices.Sort(registered)
	for _, key := range uncovered {
		if !slices.Contains(registered, key) {
			t.Errorf("%s has no index its parent's delete can use and is not in the register.\n"+
				"Index the referencing columns in the constraint's own order, or add the line and "+
				"say in the PR why the scan is affordable there.", key)
		}
	}
	for _, key := range registered {
		if !slices.Contains(uncovered, key) {
			t.Errorf("%s is in the register but is indexed now — delete its line, so the register "+
				"keeps meaning what it says.", key)
		}
	}
}

var foreignKeyIndexDebt = []string{
	"activity (assignee_id)",
	"activity (host_user_id)",
	"activity (kind)",
	"activity (source_author_id)",
	"agent_run (approval_id)",
	"agent_run (passport_id)",
	"agent_standing_grant (passport_id, user_id)",
	"ai_call (config_hash)",
	"ai_task_run (passport_id)",
	"approval (decided_by)",
	"approval (on_behalf_of)",
	"approval (passport_id)",
	"approval (staged_by_connection)",
	"assurance_resolution (actor_id)",
	"attachment (contract_id)",
	"attachment (supersedes_id)",
	"audit_log (on_behalf_of)",
	"automation (owner_id)",
	"capture_connection (context_tag_id)",
	"capture_freemail_domain (created_by)",
	"capture_pending_counterparty (proposal_id)",
	"capture_thread_verdict (first_activity_id)",
	"capture_thread_verdict (user_id)",
	"channel_connection (connected_by)",
	"commission_entry (reversal_of)",
	"comms_outbound (link_id)",
	"comms_outbound (user_id)",
	"communication_basis (source_activity_id)",
	"communication_decision (instruction_id)",
	"communication_instruction (revoked_by)",
	"communication_review (superseded_by)",
	"communication_suppression (carried_from)",
	"communication_suppression (purpose_id)",
	"company (merged_into_id)",
	"company (owner_id)",
	"company (source_author_id)",
	"company_domain_disposition (owner_id)",
	"company_domain_disposition (site_read_id)",
	"company_fact (site_read_id)",
	"confirm_token (purpose_id)",
	"consent_doi_token (purpose_id)",
	"consent_event (consent_text_version_id)",
	"consent_event (purpose_id)",
	"consent_text_version (purpose_id)",
	"contact (owner_id)",
	"contact (source_author_id)",
	"contact_channel_identity (provider)",
	"contact_consent (purpose_id)",
	"contact_phone (superseded_phone_id)",
	"contract (company_id)",
	"contract (deal_id)",
	"contract (project_id)",
	"contract (superseded_by_id)",
	"conversation_claim (corrected_by_user_id)",
	"conversation_claim (task_activity_id)",
	"custom_field (created_by)",
	"data_subject_request (assignee_id)",
	"data_subject_request (contact_id)",
	"deal (acquisition_source)",
	"deal (company_id)",
	"deal (id, arr_source_offer_id)",
	"deal (owner_id)",
	"deal (partner_company_id)",
	"deal (pipeline_id)",
	"deal (project_id)",
	"deal (source_author_id)",
	"deal (stage_id)",
	"deal (stage_id, pipeline_id)",
	"deal_correction (reversal_audit_id)",
	"deal_correction (run_id)",
	"deal_room (steward_user_id)",
	"deal_room_comment (author_participant_id, room_id)",
	"deal_room_comment (author_user_id)",
	"deal_room_document (attachment_id)",
	"deal_room_engagement (document_id, room_id)",
	"deal_room_participant (invited_by)",
	"deal_room_thread (attachment_id)",
	"deal_room_thread (author_participant_id, room_id)",
	"deal_room_thread (author_user_id)",
	"deal_room_thread (resolved_by_user_id)",
	"deal_stage_evidence (contradicted_by)",
	"deal_stage_evidence (criterion_id)",
	"deal_stage_evidence (refuted_by)",
	"deal_stage_history (from_stage_id)",
	"deal_stage_history (reversal_of)",
	"deal_stage_history (to_stage_id)",
	"dedupe_candidate (disposed_by)",
	"field_mask (object, field)",
	"finance_customer_link (company_id)",
	"finance_customer_link (connection_id)",
	"forecast_call (author_id)",
	"forecast_call (supersedes_id)",
	"forecast_snapshot (call_id)",
	"forecast_snapshot (pipeline_id)",
	"intro_request (introducer_user_id)",
	"intro_request (requester_user_id)",
	"intro_request (source_activity_id)",
	"intro_request (suggested_user_id)",
	"intro_request (through_contact_id)",
	"lead (owner_id)",
	"lead (project_id)",
	"lead (promoted_contact_id)",
	"lead (source_author_id)",
	"lead_manual_signal (set_by)",
	"linkedin_connection (matched_company_id)",
	"list (owner_id)",
	"list (team_id)",
	"meeting_invitation (host_user_id)",
	"meeting_proposal (host_user_id)",
	"meeting_proposal (invitation_id)",
	"oauth_grant (client_id)",
	"offer (buyer_company_id)",
	"offer_line_item (product_id)",
	"onboarding_wizard_state (site_read_id)",
	"passport (granted_by)",
	"preference_token (contact_email_id)",
	"privacy_notice_case (owner_user_id)",
	"privacy_notice_case (resolved_by)",
	"project (company_id)",
	"project (owner_id)",
	"project (source_author_id)",
	"project_health_assessment (project_id, supersedes_assessment_id)",
	"provider_connection (connected_by)",
	"provider_employment_resolution (company_id)",
	"provider_employment_resolution (relationship_id)",
	"provider_run (requested_by)",
	"record_assignment (role_id)",
	"record_assignment (team_id)",
	"record_assignment (user_id)",
	"record_grant (granted_by)",
	"report_definition (audience_team_id)",
	"report_definition (owner_id)",
	"report_definition_revision (created_by)",
	"report_execution (owner_id)",
	"report_execution (report_id, report_revision)",
	"report_execution (schedule_id)",
	"report_schedule (owner_id)",
	"report_schedule (report_id, report_revision)",
	"reporting_framework_revision (created_by)",
	"runner_job (agent_run_id)",
	"runner_job (passport_id)",
	"sales_target (pipeline_id)",
	"sales_target_revision (created_by)",
	"scheduled_send (activity_id)",
	"scheduled_send (agent_on_behalf_of)",
	"scheduled_send (delivery_id)",
	"sdr_handoff (assigned_to)",
	"sdr_handoff (company_id)",
	"sdr_handoff (deal_id)",
	"sdr_handoff (reason_id, reason_applies_to)",
	"sdr_handoff_event (reason_id, reason_applies_to)",
	"signal (owner_id)",
	"signal (resolved_contact_id)",
	"signal_resolution (matched_company_id)",
	"signal_resolution (resolved_by)",
	"signal_thread_scan (resolved_company_id)",
	"stage_progression_outcome (reversed_by)",
	"stage_progression_policy (enabled_by)",
	"system_log (on_behalf_of)",
	"team (parent_team_id)",
	"voice_build (voice_profile_id, result_version)",
	"voice_learning_signal (voice_profile_id, profile_version)",
	"voice_profile_delta (voice_profile_id, from_version)",
	"voice_profile_version (voice_profile_id, predecessor_version)",
	"weekly_plan_commitment (manager_user_id)",
	"weekly_review (prior_review_id)",
}
