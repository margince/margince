// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// machineCreatedSQL is when Margince itself created the record: the earliest
// create row by the system or an agent with no passport. An agent a colleague
// runs carries a passport, and its work is that colleague's own. min() makes a
// replayed create count once.
func machineCreatedSQL(entityType, rowAlias string) string {
	return fmt.Sprintf(`SELECT min(c.occurred_at) AS at FROM audit_log c
 WHERE c.entity_type = '%s' AND c.entity_id = %s.id AND c.action = 'create'
   AND c.actor_type IN ('system', 'agent') AND c.passport_id IS NULL`, entityType, rowAlias)
}

// byAColleagueSQL is whether an audit row was a colleague's own act: their session,
// or an agent working under their passport. The system and a passportless
// agent are rules.
func byAColleagueSQL(alias string) string {
	return fmt.Sprintf(`(%[1]s.actor_type = 'human' OR (%[1]s.actor_type = 'agent' AND %[1]s.passport_id IS NOT NULL))`, alias)
}

// taskMovedSQL reads the first move off the first holder: by a colleague it says
// the task was not theirs, by a rule it was rerouted. It holds even when the
// task later came back to them.
func taskMovedSQL() string {
	return fmt.Sprintf(`WHEN mv.actor_type IS NOT NULL AND %s THEN 'not_mine'
         WHEN mv.actor_type IS NOT NULL THEN 'rerouted'`, byAColleagueSQL("mv"))
}

// automaticItemsSQL is one row per automatic item created in the window: its
// kind, first colleague, creation instant and current outcome. An item decided
// days later still counts on its creation day.
//
// An approval has no verb for a colleague to move it, so any move is a rule's.
// One the system applied by itself waited on nobody and is left out.
func automaticItemsSQL(arg func(any) int, from, to time.Time) string {
	unscrubbed := privacy.UnscrubbedImageSQL("h", fmt.Sprintf("$%d", arg(privacy.ScrubVerbs())))
	fromPos, toPos := arg(from), arg(to)
	return fmt.Sprintf(`
SELECT CASE a.source_system WHEN $%[1]d THEN '%[2]s' ELSE '%[3]s' END AS kind,
       COALESCE(mv.holder, a.assignee_id) AS colleague, made.at,
       CASE %[4]s
         WHEN a.is_done THEN 'completed'
         WHEN a.archived_at IS NOT NULL AND %[11]s THEN 'dismissed'
         WHEN a.archived_at IS NOT NULL THEN 'expired'
         ELSE 'open' END AS outcome
  FROM activity a
 CROSS JOIN LATERAL (%[5]s) made
  LEFT JOIN LATERAL (%[6]s) mv ON true
  LEFT JOIN LATERAL (SELECT x.actor_type, x.passport_id FROM audit_log x
                      WHERE x.entity_type = 'activity' AND x.entity_id = a.id AND x.action = 'archive'
                      ORDER BY x.occurred_at DESC, x.id DESC LIMIT 1) last_archive ON true
 WHERE a.kind = 'task' AND a.source_system = ANY($%[7]d)
   AND made.at >= $%[9]d AND made.at < $%[10]d
UNION ALL
SELECT '%[8]s', COALESCE(mv.holder, ap.requested_decider_id), made.at,
       CASE WHEN mv.actor_type IS NOT NULL THEN 'rerouted'
         WHEN ap.status = 'approved' THEN 'completed'
         WHEN ap.status = 'rejected' THEN 'rejected'
         WHEN ap.status = 'expired' THEN 'expired'
         ELSE 'open' END
  FROM approval ap
 CROSS JOIN LATERAL (%[12]s) made
  LEFT JOIN LATERAL (%[13]s) mv ON true
 WHERE NOT ap.decided_by_system AND made.at >= $%[9]d AND made.at < $%[10]d`,
		arg(activities.CommitmentTaskSource),
		crmcontracts.AutomaticWorkKindNamePromiseTask, crmcontracts.AutomaticWorkKindNameReplyReminder,
		taskMovedSQL(),
		machineCreatedSQL("activity", "a"),
		firstMoveSQL("activity", "a", "assignee_id", unscrubbed),
		arg([]string{activities.CommitmentTaskSource, activities.EmailRequestTaskSource}),
		crmcontracts.AutomaticWorkKindNameApprovalRequest,
		fromPos, toPos,
		byAColleagueSQL("last_archive"),
		machineCreatedSQL("approval", "ap"),
		firstMoveSQL("approval", "ap", "requested_decider_id", unscrubbed))
}

// automaticWorkRow is one (colleague, kind, day, outcome) count. A nil colleague is
// an item whose colleague no longer exists.
type automaticWorkRow struct {
	colleague *ids.UUID
	kind      crmcontracts.AutomaticWorkKindName
	day       time.Time
	outcome   string
	count     int
}

func readAutomaticWorkRows(ctx context.Context, tx pgx.Tx, zone string, from, to time.Time) ([]automaticWorkRow, error) {
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	items := automaticItemsSQL(arg, from, to)
	query := fmt.Sprintf(`SELECT item.colleague, item.kind, (item.at AT TIME ZONE $%d)::date, item.outcome, count(*)
  FROM (%s) item GROUP BY 1, 2, 3, 4`, arg(zone), items)
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("automatic work: reading the items: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (automaticWorkRow, error) {
		var r automaticWorkRow
		err := row.Scan(&r.colleague, &r.kind, &r.day, &r.outcome, &r.count)
		return r, err
	})
}

// readWaitingNotMine counts, per colleague, the waiting messages they marked
// "Not mine" in the window. A message marked, picked up and marked again
// counts once.
func readWaitingNotMine(ctx context.Context, tx pgx.Tx, from, to time.Time) (map[ids.UUID]int, error) {
	rows, err := tx.Query(ctx, `
		SELECT u.id, count(DISTINCT l.entity_id)
		  FROM app_user u
		  JOIN audit_log l ON l.actor_id = $1 || u.id::text
		 WHERE l.actor_type = 'human' AND l.entity_type = 'activity' AND l.action = 'update'
		   AND l.after->>'disposition' = $2 AND l.occurred_at >= $3 AND l.occurred_at < $4
		 GROUP BY u.id`,
		principal.HumanIDPrefix, string(crmcontracts.NotMine), from, to)
	if err != nil {
		return nil, fmt.Errorf("automatic work: reading the Not mine marks: %w", err)
	}
	marks := map[ids.UUID]int{}
	var user ids.UUID
	var count int
	if _, err := pgx.ForEachRow(rows, []any{&user, &count}, func() error {
		marks[user] = count
		return nil
	}); err != nil {
		return nil, fmt.Errorf("automatic work: decoding the Not mine marks: %w", err)
	}
	return marks, nil
}

type automaticWorkSeat struct {
	id     ids.UUID
	name   string
	status crmcontracts.AutomaticWorkColleagueStatus
}

// readAutomaticWorkSeats reads the active colleagues, plus any other seat in
// `named`: a deactivated seat that was handed work in the window still shows.
func readAutomaticWorkSeats(ctx context.Context, tx pgx.Tx, named []ids.UUID) ([]automaticWorkSeat, error) {
	rows, err := tx.Query(ctx, `
		SELECT id, display_name, status FROM app_user
		 WHERE NOT is_agent AND (status = 'active' OR id = ANY($1))
		 ORDER BY lower(display_name), id`, named)
	if err != nil {
		return nil, fmt.Errorf("automatic work: reading the colleagues: %w", err)
	}
	return pgx.CollectRows(rows, func(row pgx.CollectableRow) (automaticWorkSeat, error) {
		var s automaticWorkSeat
		err := row.Scan(&s.id, &s.name, &s.status)
		return s, err
	})
}
