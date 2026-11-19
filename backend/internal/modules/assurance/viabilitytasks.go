// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// answeredTaskSQL admits a task (column `task`) whose every filed finding is
// one ViabilityAnsweredTypes names, on a deal with a judged receipt under the
// current policy. Such a task asks what no pass files a task for any more.
const answeredTaskSQL = `NOT EXISTS (
	SELECT 1 FROM assurance_task_item i JOIN assurance_exception e ON e.id = i.exception_id
	 WHERE i.task_activity_id = task
	   AND (e.type <> ALL ($1) OR NOT EXISTS (SELECT 1 FROM assurance_deal_receipt v
	                                          WHERE v.deal_id = i.subject_id AND v.policy_version = $2
	                                            AND v.outcome <> $3)))`

// ViabilityAnsweredTasksTx pages the tasks the input check filed, by id after the given
// one. It answers every task it read, and those whose findings the viability
// decision answers. Whether a task is still open is the activity's fact, which
// this module does not read.
func ViabilityAnsweredTasksTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (read, answered []ids.UUID, err error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionRead); err != nil {
		return nil, nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT task, `+answeredTaskSQL+`
		  FROM (SELECT DISTINCT task_activity_id AS task FROM assurance_task_item
		         WHERE task_activity_id > $4 ORDER BY task_activity_id LIMIT $5) page
		 ORDER BY task`,
		ViabilityAnsweredTypes(), viability.PolicyVersion, string(viability.OutcomeUnknown), after, limit)
	if err != nil {
		return nil, nil, fmt.Errorf("assurance: reading the tasks the viability decision answers: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var task ids.UUID
		var isAnswered bool
		if err := rows.Scan(&task, &isAnswered); err != nil {
			return nil, nil, err
		}
		read = append(read, task)
		if isAnswered {
			answered = append(answered, task)
		}
	}
	return read, answered, rows.Err()
}

// CountViabilityAnsweredTasks counts the tasks ViabilityAnsweredTasksTx selects, reading at most
// maximum plus one.
func CountViabilityAnsweredTasks(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64,
) (int64, error) {
	if err := auth.Require(ctx, rbacBundle, principal.ActionRead); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT task FROM
		(SELECT DISTINCT task_activity_id AS task FROM assurance_task_item) tasks
		 WHERE `+answeredTaskSQL+` LIMIT $4) answered`,
		ViabilityAnsweredTypes(), viability.PolicyVersion, string(viability.OutcomeUnknown), maximum+1).Scan(&count)
	return count, err
}
