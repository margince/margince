// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// qualifyTaskCodes are the templates a qualify task's subject is written from,
// each with the lead's name as its one label.
var qualifyTaskCodes = []string{"lead.reply", "lead.booked", "lead.held"}

// qualifyTasksWithoutTextSQL is the qualify tasks written before the subject
// carried its template, whose subject no human or agent has changed since.
const qualifyTasksWithoutTextSQL = `FROM activity t WHERE t.kind = 'task' AND t.source_system = $1
   AND t.system_text IS NULL AND t.subject IS NOT NULL AND t.archived_at IS NULL AND t.restricted_at IS NULL
   AND NOT EXISTS (SELECT 1 FROM audit_log edit WHERE edit.entity_type = 'activity' AND edit.entity_id = t.id
     AND edit.action <> 'create' AND edit.actor_type IN ('human', 'agent') AND edit.after ? 'subject')`

// DescribeQualifyTaskTextTx gives each unedited qualify task on the page after
// `after` the template its English subject was written from, so it renders in
// each reader's language. A subject no template renders stays as it is.
func DescribeQualifyTaskTextTx(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) ([]ids.UUID, int, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return nil, 0, err
	}
	rows, err := tx.Query(ctx, `SELECT t.id, t.subject, t.version `+qualifyTasksWithoutTextSQL+` AND t.id > $2 ORDER BY t.id LIMIT $3 FOR UPDATE OF t`,
		provenance.LeadRepliedSource, after, limit)
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading qualify tasks without system text: %w", err)
	}
	type task struct {
		id      ids.UUID
		subject string
		version int64
	}
	page, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (task, error) {
		var one task
		err := row.Scan(&one.id, &one.subject, &one.version)
		return one, err
	})
	if err != nil {
		return nil, 0, fmt.Errorf("activities: reading qualify tasks without system text: %w", err)
	}
	scanned, described := make([]ids.UUID, 0, len(page)), 0
	for _, one := range page {
		scanned = append(scanned, one.id)
		message := qualifySubjectText(one.subject)
		if message == nil {
			continue
		}
		// The read locked the row, so a decline is only counted as not described.
		tag, err := tx.Exec(ctx, `UPDATE activity SET system_text = $2 WHERE id = $1 AND version = $3 AND archived_at IS NULL`,
			one.id, message, one.version)
		if err != nil {
			return nil, 0, fmt.Errorf("activities: describing a qualify task's subject: %w", err)
		}
		if tag.RowsAffected() == 1 {
			described++
		}
	}
	return scanned, described, nil
}

// qualifySubjectText is the bound template that renders subject, or nil.
func qualifySubjectText(subject string) *systemtext.Message {
	for _, code := range qualifyTaskCodes {
		prefix, suffix, ok := strings.Cut(systemtext.Copy(code, textlang.English), "%s")
		if !ok || !strings.HasPrefix(subject, prefix) || !strings.HasSuffix(subject, suffix) || len(subject) <= len(prefix)+len(suffix) {
			continue
		}
		name := subject[len(prefix) : len(subject)-len(suffix)]
		bound, err := systemtext.Bind(&systemtext.Message{Version: 1, Subject: systemtext.Sentence(code, systemtext.Label(name))}, subject, "")
		if err == nil && bound != nil && bound.Subject != nil {
			return bound
		}
	}
	return nil
}

// CountQualifyTasksWithoutText caps the estimate at the tasks the batch reads.
func CountQualifyTasksWithoutText(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT t.id `+qualifyTasksWithoutTextSQL+` LIMIT $2) candidates`,
		provenance.LeadRepliedSource, maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}
