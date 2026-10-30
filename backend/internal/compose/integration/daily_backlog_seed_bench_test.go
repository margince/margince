// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration && bench

package integration

// The backlog the Worklist sizes are measured over, in the daily corpus's own
// shapes. A task is written the way the corpus writes one. A customer's
// unanswered mail is written like the last message of a waiting thread.

// backlogMix is the share of a backlog, in percent, each non-task source holds;
// open tasks take the rest. The dominant-source variant is all tasks.
type backlogMix struct {
	name           string
	waiting, deals int
}

var (
	backlogMixed    = backlogMix{name: "mixed", waiting: 25, deals: 15}
	backlogDominant = backlogMix{name: "tasks_only"}
)

// backlog writes candidates [from, to) for one rep. Tasks are overdue by one to
// twenty-five days. Inbound mail from the rep's contacts has no answer. Open
// deals have been idle for sixty days. Every age sits inside the window its
// lane reads, so each row is admitted work.
func (s dailySeeder) backlog(rep, pipeline string, mix backlogMix, from, to int) {
	s.t.Helper()
	s.exec("backlog contacts", `CREATE TEMP TABLE IF NOT EXISTS backlog_contact AS
	      SELECT 0::bigint AS n, NULL::uuid AS rep, NULL::uuid AS id, NULL::text AS email, NULL::uuid AS company_id WITH NO DATA`)
	s.exec("backlog contacts", `TRUNCATE backlog_contact`)
	s.exec("backlog contacts", `INSERT INTO backlog_contact
	      SELECT row_number() OVER (ORDER BY p.id) - 1, p.owner_id, p.id, ce.email, r.company_id
	      FROM contact p JOIN contact_email ce ON ce.contact_id = p.id AND ce.is_primary
	      JOIN relationship r ON r.contact_id = p.id AND r.kind = 'employment' AND r.is_current_primary
	      WHERE p.owner_id = $1::uuid AND p.archived_at IS NULL`, rep)
	s.exec("backlog ordinals", `CREATE TEMP TABLE backlog_row AS
	      SELECT i, public.uuidv7() AS id,
	             CASE WHEN i % 100 < $3 THEN 'waiting' WHEN i % 100 < $3 + $4 THEN 'deal' ELSE 'task' END AS kind,
	             c.id AS contact_id, c.email, c.company_id, c.rep,
	             now() - (1 + i % 25) * interval '1 day' - (i % 600) * interval '1 minute' AS at
	      FROM generate_series($1::int, $2::int - 1) AS i
	      JOIN backlog_contact c ON c.n = i % (SELECT count(*) FROM backlog_contact)`, from, to, mix.waiting, mix.deals)
	s.backlogActivities()
	s.backlogDeals(pipeline)
	s.exec("backlog drop", `DROP TABLE backlog_row`)
}

func (s dailySeeder) backlogActivities() {
	s.execDeferringTriggers("backlog activity", "activity", []string{settlementEvidenceTrigger}, `INSERT INTO activity (id, kind, subject, body, occurred_at, due_at, assignee_id, direction,
	                                         source_system, source_id, source, captured_by, thread_key, audience, language, created_at)
	      SELECT b.id, CASE b.kind WHEN 'task' THEN 'task' ELSE 'email' END,
	             CASE b.kind WHEN 'task' THEN 'Rückruf: Angebot ' || b.i ELSE 'Frage zum Angebot ' || b.i END,
	             'Backlog ' || b.i, b.at, CASE WHEN b.kind = 'task' THEN b.at END,
	             CASE WHEN b.kind = 'task' THEN b.rep END, CASE WHEN b.kind = 'waiting' THEN 'inbound' END,
	             CASE WHEN b.kind = 'waiting' THEN 'email' END,
	             CASE WHEN b.kind = 'waiting' THEN 'backlog-' || b.rep || '-' || b.i || '@bench.example' END,
	             CASE WHEN b.kind = 'waiting' THEN 'email' ELSE 'manual' END, 'human:' || b.rep,
	             CASE WHEN b.kind = 'waiting' THEN 'backlog-thread-' || b.rep || '-' || b.i || '@bench.example' END,
	             'workspace', 'de', b.at
	      FROM backlog_row b WHERE b.kind <> 'deal' ORDER BY b.i`)
	s.exec("backlog capture_import", `INSERT INTO capture_import (activity_id, user_id, imported_at)
	      SELECT b.id, b.rep, b.at FROM backlog_row b WHERE b.kind = 'waiting' ORDER BY b.i`)
	s.execDeferringTriggers("backlog participant", "activity_participant", []string{settlementEvidenceTrigger}, `INSERT INTO activity_participant (activity_id, user_id, contact_id, address, role, display_name)
	      SELECT b.id, x.user_id, x.contact_id, x.address, x.role, NULL
	      FROM backlog_row b JOIN app_user u ON u.id = b.rep
	      CROSS JOIN LATERAL (VALUES (b.rep, NULL::uuid, NULL::text, 'to'), (NULL::uuid, NULL::uuid, u.email, 'to'),
	                                 (NULL::uuid, b.contact_id, b.email, 'from')) AS x(user_id, contact_id, address, role)
	      WHERE b.kind = 'waiting'`)
	s.execDeferringTriggers("backlog link", "activity_link", []string{"activity_link_last_activity", settlementEvidenceTrigger}, `INSERT INTO activity_link (activity_id, entity_type, contact_id, company_id, deal_id)
	      SELECT b.id, 'contact', b.contact_id, NULL::uuid, NULL::uuid FROM backlog_row b WHERE b.kind <> 'deal'
	      UNION ALL
	      SELECT b.id, 'company', NULL, b.company_id, NULL FROM backlog_row b WHERE b.kind <> 'deal'`)
}

// backlogDeals are open deals at the contact's employer, created sixty days ago.
// Nothing touched them since and the close date is ahead, so they are only quiet.
func (s dailySeeder) backlogDeals(pipeline string) {
	s.exec("backlog deal", `INSERT INTO deal (id, name, amount_minor, amount_minor_base, currency, pipeline_id, stage_id,
	                                 company_id, owner_id, status, expected_close_date, forecast_category, priority,
	                                 commercial_motion, source, captured_by, created_at)
	      SELECT b.id, 'Backlog deal ' || b.i, 500000, 500000,
	             coalesce((SELECT value #>> '{}' FROM setting WHERE key = 'installation.base_currency'), 'EUR'),
	             $1::uuid, (SELECT s.id FROM stage s WHERE s.pipeline_id = $1::uuid AND s.semantic = 'open'
	                        AND s.archived_at IS NULL ORDER BY s.position LIMIT 1),
	             b.company_id, b.rep, 'open', current_date + 30, 'pipeline', 'medium',
	             'new_business', 'manual', 'human:' || b.rep, now() - interval '60 days'
	      FROM backlog_row b WHERE b.kind = 'deal' ORDER BY b.i`, pipeline)
}
