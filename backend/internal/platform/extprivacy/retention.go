// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

// What retention and legal hold mean for the rows of an extension's tables. A
// row has no hold of its own: it is held while a core record it links to is.
// The audit row and the event stay the caller's, as for clearing.

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/retentionscope"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// Swept reports whether a retention sweep can reach the table's rows: only a
// declared age says when a row is old.
func Swept(t privacy.Table) bool { return t.AgeColumn != "" }

// SweptTables returns the declared tables a retention sweep can reach.
func (r Registry) SweptTables() []privacy.Table {
	var out []privacy.Table
	for _, t := range r.tables {
		if Swept(t) {
			out = append(out, t)
		}
	}
	return out
}

// Window bounds which rows of one table a sweep may take.
type Window struct {
	// OlderThan: only a row whose age column is before it is due.
	OlderThan time.Time
}

// statement collects the arguments of one query, so every placeholder is
// derived from the argument it stands for.
type statement struct{ args []any }

//craft:ignore naked-any a query argument is whatever pgx encodes for its column, as Exec and Query take it
func (s *statement) arg(v any) string {
	s.args = append(s.args, v)
	return "$" + strconv.Itoa(len(s.args))
}

// heldSQL is true for a row of the table aliased `t` that links to a held core
// record. An activity is held through any record it is linked to, as the core
// sweep and erasure read it.
func heldSQL(t privacy.Table) string {
	var arms []string
	for _, l := range t.Links {
		column := "t." + pgx.Identifier{l.Column}.Sanitize()
		if l.Record == privacy.Activity {
			arms = append(arms, retentionscope.HeldThroughAnyLink(column))
			continue
		}
		arms = append(arms, `EXISTS (SELECT 1 FROM `+pgx.Identifier{string(l.Record)}.Sanitize()+
			` k WHERE k.id = `+column+` AND k.legal_hold)`)
	}
	if len(arms) == 0 {
		return "false"
	}
	return "(" + strings.Join(arms, " OR ") + ")"
}

// dueSQL is what the window allows over the table aliased `t`, less the held
// rows. A row whose link is still empty is due by age alone.
func (s *statement) dueSQL(t privacy.Table, w Window) string {
	return "t." + pgx.Identifier{t.AgeColumn}.Sanitize() + ` < ` + s.arg(w.OlderThan) + ` AND NOT ` + heldSQL(t)
}

// Due returns up to limit rows of a table the window lets a sweep take, oldest
// first. For a sweep that clears, a row already blank is not due, so the sweep
// never picks one row twice.
func (r Registry) Due(ctx context.Context, tx pgx.Tx, t privacy.Table, w Window, clears bool, limit int) ([]ids.UUID, error) {
	var s statement
	where := s.dueSQL(t, w)
	if clears {
		set, err := clearAssignments(ctx, tx, t)
		if err != nil {
			return nil, err
		}
		where += " AND (" + set.notBlank + ")"
	}
	query := `SELECT t.id FROM ` + qualified(t.Name) + ` t WHERE ` + where +
		` ORDER BY t.` + pgx.Identifier{t.AgeColumn}.Sanitize() + `, t.id LIMIT ` + s.arg(limit)
	rows, err := tx.Query(ctx, query, s.args...)
	if err != nil {
		return nil, fmt.Errorf("extprivacy: selecting the rows of %s past their window: %w", t.Name, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// ClearDue clears the personal columns of one row, asking the window and the
// holds again in the same statement. It reports whether the row changed: a row
// held, cleared or gone since the selection is left alone.
func (r Registry) ClearDue(ctx context.Context, tx pgx.Tx, t privacy.Table, w Window, id ids.UUID) (bool, error) {
	set, err := clearAssignments(ctx, tx, t)
	if err != nil {
		return false, err
	}
	var s statement
	query := `UPDATE ` + qualified(t.Name) + ` t SET ` + set.assignments +
		` WHERE t.id = ` + s.arg(id) + ` AND (` + set.notBlank + `) AND ` + s.dueSQL(t, w)
	tag, err := tx.Exec(ctx, query, s.args...)
	if err != nil {
		return false, fmt.Errorf("extprivacy: clearing a row of %s: %w", t.Name, err)
	}
	return oneRow(t, id, tag.RowsAffected())
}

// DeleteDue deletes one row on the same terms as ClearDue.
func (r Registry) DeleteDue(ctx context.Context, tx pgx.Tx, t privacy.Table, w Window, id ids.UUID) (bool, error) {
	var s statement
	query := `DELETE FROM ` + qualified(t.Name) + ` t WHERE t.id = ` + s.arg(id) + ` AND ` + s.dueSQL(t, w)
	tag, err := tx.Exec(ctx, query, s.args...)
	if err != nil {
		return false, fmt.Errorf("extprivacy: deleting a row of %s: %w", t.Name, err)
	}
	return oneRow(t, id, tag.RowsAffected())
}

// oneRow reads what an act on one id changed. More than one row is an error, so
// the caller's transaction rolls back: one certificate cannot stand for two rows.
func oneRow(t privacy.Table, id ids.UUID, affected int64) (bool, error) {
	if affected > 1 {
		return false, fmt.Errorf("extprivacy: %d rows of %s carry id %s; an id must name one row", affected, t.Name, id)
	}
	return affected == 1, nil
}
