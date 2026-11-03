// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Every foreign key says what a parent delete does to the child.
//
// Omitting the clause is not neutral: Postgres reads it as NO ACTION, so the row
// refuses the delete anyway — but nobody wrote that down, and the next reader cannot
// tell a considered refusal from a clause somebody forgot. The three the tree does
// write mean different things: CASCADE for a row the parent owns, SET NULL for an
// attribution the record outlives, RESTRICT for a catalog parent nothing may orphan.
//
// Postgres normalises an explicit ON DELETE NO ACTION away, so a considered one and a
// forgotten clause leave the same catalog: confdeltype reads 'a' for both, and so does
// pg_get_constraintdef — verified by writing one and watching this gate still report
// the key.
//
// So the rule is a NON-DEFAULT clause. A key that genuinely means "refuse the delete"
// writes RESTRICT, which is recorded, and which also checks the parent row
// immediately rather than deferring to the end of the statement.

import (
	"context"
	"slices"
	"testing"
)

// unwrittenDeleteRule finds the foreign keys left on the default: NO ACTION, which is
// what Postgres records both for an omitted clause and for an explicit one.
const unwrittenDeleteRule = `
SELECT c.conrelid::regclass::text || ' (' ||
       (SELECT string_agg(att.attname, ', ' ORDER BY u.ord)
          FROM unnest(c.conkey) WITH ORDINALITY u(k, ord)
          JOIN pg_attribute att ON att.attrelid = c.conrelid AND att.attnum = u.k)
       || ') -> ' || c.confrelid::regclass::text
  FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = 'public'
   AND c.confdeltype = 'a'
 ORDER BY 1`

// foreignKeyCount tallies the schema's foreign keys, answered or not. It is the floor
// that stops this passing by reading nothing.
const foreignKeyCount = `
SELECT count(*) FROM pg_constraint c
  JOIN pg_class t ON t.oid = c.conrelid
  JOIN pg_namespace n ON n.oid = t.relnamespace
 WHERE c.contype = 'f' AND n.nspname = 'public'`

// TestEveryForeignKeyStatesItsDeleteRule holds the register below against the schema.
//
// Equality rather than emptiness, because 41 keys say nothing today and each needs a
// judgement rather than a default: a lookup parent means RESTRICT, an attribution
// column means SET NULL, and twelve of them are NOT NULL references to app_user where
// neither is available without either making a user undeletable or making the column
// nullable. That one is a product call, so the register carries them rather than this
// gate guessing.
//
// What it buys meanwhile is that the number cannot grow in silence: a new key with no
// clause is a finding, and one that gains a clause fails until its line goes.
func TestEveryForeignKeyStatesItsDeleteRule(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	headSchema(t, conn)
	ctx := context.Background()

	var keys int
	if err := conn.QueryRow(ctx, foreignKeyCount).Scan(&keys); err != nil {
		t.Fatalf("counting foreign keys: %v", err)
	}
	// The schema declared 475 when this landed. A collapse means the query stopped
	// recognising them, and every one would then read as answered.
	if keys < 400 {
		t.Fatalf("this gate found %d foreign key(s) and expects at least 400 — it has stopped "+
			"recognising them rather than the schema having given them up", keys)
	}

	rows, err := conn.Query(ctx, unwrittenDeleteRule)
	if err != nil {
		t.Fatalf("reading the keys that state no delete rule: %v", err)
	}
	defer rows.Close()
	var unwritten []string
	for rows.Next() {
		var one string
		if err := rows.Scan(&one); err != nil {
			t.Fatalf("scanning: %v", err)
		}
		unwritten = append(unwritten, one)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the keys that state no delete rule: %v", err)
	}

	registered := slices.Clone(unwrittenDeleteRules)
	slices.Sort(registered)
	for _, key := range unwritten {
		if !slices.Contains(registered, key) {
			t.Errorf("%s leaves a parent delete to the default and is not in the register.\n"+
				"Say what the delete does — CASCADE for a row the parent owns, SET NULL for an "+
				"attribution the record outlives, RESTRICT for a catalog parent nothing may "+
				"orphan — or add the line and say in the PR which call it is waiting on. "+
				"Writing NO ACTION is not available: Postgres normalises it away, so it reads "+
				"here exactly like the omission.", key)
		}
	}
	for _, key := range registered {
		if !slices.Contains(unwritten, key) {
			t.Errorf("%s is in the register but states a rule now — delete its line, so the "+
				"register keeps meaning what it says.", key)
		}
	}
}

// unwrittenDeleteRules are the foreign keys that state no ON DELETE clause today.
//
// A REGISTER, not a waiver list: the test above fails in both directions, so the set
// can only shrink and a new one is a finding. Each line is one judgement owed, and
// they are not all the same judgement — twelve are NOT NULL references to app_user,
// where SET NULL is unavailable and RESTRICT makes a user undeletable while any row
// names them. That is the call this issue is waiting on; the rest are derivable from
// what the parent is.
var unwrittenDeleteRules = []string{
	"activity (channel_provider) -> channel_provider",
	"activity (kind) -> activity_kind",
	"ai_call (config_hash) -> ai_call_config",
	"consent_event (consent_text_version_id) -> consent_text_version",
	"consent_event (lead_id) -> lead",
	"contact_channel_identity (provider) -> channel_provider",
	"deal (stage_id, pipeline_id) -> stage",
	"field_mask (object, field) -> maskable_field",
	"forecast_snapshot (pipeline_id) -> pipeline",
	"intro_request (introducer_user_id) -> app_user",
	"intro_request (requester_user_id) -> app_user",
	"intro_request (suggested_user_id) -> app_user",
	"intro_request (through_contact_id) -> contact",
	"lead_manual_signal (set_by) -> app_user",
	"meeting_invitation (host_user_id) -> app_user",
	"meeting_proposal (host_user_id) -> app_user",
	"provider_connection (connected_by) -> app_user",
	"provider_run (requested_by) -> app_user",
	"report_definition (audience_team_id) -> team",
	"report_definition (owner_id) -> app_user",
	"report_definition_revision (created_by) -> app_user",
	"report_definition_revision (report_id) -> report_definition",
	"report_edition (execution_id) -> report_execution",
	"report_edition (report_id) -> report_definition",
	"report_execution (owner_id) -> app_user",
	"report_execution (report_id, report_revision) -> report_definition_revision",
	"report_execution (schedule_id) -> report_schedule",
	"report_schedule (owner_id) -> app_user",
	"report_schedule (report_id, report_revision) -> report_definition_revision",
	"reporting_framework_revision (created_by) -> app_user",
	"sales_target (pipeline_id) -> pipeline",
	"sales_target_revision (created_by) -> app_user",
	"sales_target_revision (target_id) -> sales_target",
	"sdr_handoff (assigned_to) -> app_user",
	"sdr_handoff (submitted_by) -> app_user",
	"voice_build (voice_profile_id, result_version) -> voice_profile_version",
	"voice_learning_signal (voice_profile_id, profile_version) -> voice_profile_version",
	"voice_profile_delta (voice_profile_id, from_version) -> voice_profile_version",
	"voice_profile_delta (voice_profile_id, to_version) -> voice_profile_version",
	"voice_profile_version (voice_profile_id, predecessor_version) -> voice_profile_version",
	"weekly_plan_commitment (manager_user_id) -> app_user",
}
