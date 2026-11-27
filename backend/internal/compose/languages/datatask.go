// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package languages

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

const (
	// MigrationVersion is the migration that created the language table and put a
	// foreign key on every language column.
	MigrationVersion = "core/1795744592"

	// ReasonNotRegistered names a stored value the language table does not hold.
	// Its sentence and remedy are in systemtext.
	ReasonNotRegistered = "language_not_registered"
	// ReasonBadLocale names a stored locale that is not a language with an
	// optional region. Its sentence and remedy are in systemtext.
	ReasonBadLocale = "locale_shape_invalid"

	notRegistered = `t.%[1]s IS NOT NULL AND NOT EXISTS (SELECT 1 FROM language l WHERE l.code = t.%[1]s)`
	badLocale     = `t.%[1]s IS NOT NULL AND t.%[1]s !~ '^[a-z]{2}(-[A-Z]{2})?$'`
)

// column is a language column held by the language table's foreign key, or a
// locale column held by a shape check. Identifiers are literals, never request text.
type column struct {
	table, name, id string
	locale          bool
}

// columns lists every column the task checks. The language-column gate holds
// Checked to the head catalog, so a column added there cannot be left out.
//
//nolint:goconst // each name is that table's own column, not one shared word
var columns = []column{
	{table: "activity", name: "language", id: "id"},
	{table: "app_user", name: "locale", id: "id"},
	{table: "confirm_token", name: "question_locale", id: "id"},
	{table: "consent_text_version", name: "locale", id: "id"},
	{table: "team_weekly_review", name: "summary_language", id: "id"},
	{table: "legal_entity", name: "default_language", id: "id"},
	{table: "offer", name: "language", id: "id"},
	// A product's translations have no row id of their own, so the product is
	// what a reader is pointed to.
	{table: "product_translation", name: "language", id: "product_id"},
	{table: "offer_template", name: "locale", id: "id", locale: true},
	{table: "lead_privacy_notice", name: "locale", id: "id", locale: true},
}

// DataTask reports each stored value the language table or a locale shape would
// refuse. It changes no row and guesses no mapping. A key added not valid leaves
// older rows unchecked, and an admin reads the report to decide what each means.
func DataTask() datatask.Task {
	steps := make([]datatask.Step, len(columns))
	for i, c := range columns {
		steps[i] = c.step()
	}
	return datatask.Task{
		ID: "core/1795713760_unknown_language_values", Version: 1, Requires: []string{MigrationVersion}, Steps: steps,
		Title: systemtext.Literal("datatask.title.core.1795713760_unknown_language_values"),
	}
}

// Checked lists the columns the task reports on, as table.column.
func Checked() []string {
	out := make([]string, len(columns))
	for i, c := range columns {
		out[i] = c.table + "." + c.name
	}
	return out
}

func (c column) step() datatask.Step {
	return datatask.Step{
		Name: c.table + "_" + c.name, Limit: 500, MinLimit: 10,
		Audit:    datatask.AuditPolicy{Reason: "the step only reports rows and writes none"},
		Estimate: c.estimate, Batch: c.batch,
	}
}

func (c column) reason() string {
	if c.locale {
		return ReasonBadLocale
	}
	return ReasonNotRegistered
}

func (c column) violates() string {
	pattern := notRegistered
	if c.locale {
		pattern = badLocale
	}
	return fmt.Sprintf(pattern, c.name)
}

func (c column) estimate(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT 1 FROM `+c.table+` t WHERE `+c.violates()+` LIMIT $1) found`,
		maximum+1).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func (c column) batch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT DISTINCT t.`+c.id+` FROM `+c.table+` t WHERE `+c.violates()+
		` AND t.`+c.id+` > $1 ORDER BY t.`+c.id+` LIMIT $2`, after, in.Limit)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	out, err := datatask.AdvanceUUID(in, found, 0)
	if err != nil || len(found) == 0 {
		return out, err
	}
	action := &datatask.ActionNeeded{Reason: c.reason()}
	for _, id := range found {
		action.Records = append(action.Records, datatask.RecordRef{Type: c.table, ID: id})
	}
	out.Action = action
	return out, nil
}
