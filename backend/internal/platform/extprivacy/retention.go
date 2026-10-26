// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package extprivacy

// What retention and legal hold do to the rows of an extension's tables. The
// reads and writes are here, and the audit row and the event are the caller's,
// as for clearing.

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/privacy"
)

// Table returns the declaration of one table, or false when no unit declared it.
func (r Registry) Table(name string) (privacy.Table, bool) {
	for _, t := range r.tables {
		if t.Name == name {
			return t, true
		}
	}
	return privacy.Table{}, false
}

// Held lists the rows that name one of the subjects and are under legal hold.
func (r Registry) Held(ctx context.Context, tx pgx.Tx, subjects map[privacy.RecordKind][]ids.UUID) ([]Cleared, error) {
	var held []Cleared
	err := r.forEachLink(subjects, func(lc linkedColumn, records []ids.UUID) error {
		if !identifiable(lc.table) {
			return nil
		}
		rows, err := tx.Query(ctx, `SELECT t.id FROM `+qualified(lc.table.Name)+` t
			JOIN extension_legal_hold h ON h.table_name = $2 AND h.row_id = t.id
			WHERE t.`+pgx.Identifier{lc.column}.Sanitize()+` = ANY($1)`, records, lc.table.Name)
		if err != nil {
			return fmt.Errorf("extprivacy: reading the held rows of %s: %w", lc.table.Name, err)
		}
		found, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return fmt.Errorf("extprivacy: reading the held rows of %s: %w", lc.table.Name, err)
		}
		for _, id := range found {
			held = append(held, Cleared{Table: lc.table.Name, ID: id})
		}
		return nil
	})
	return held, err
}

// RowState is what a retention decision on one row reads in the transaction
// that acts on it.
type RowState struct {
	Held    bool
	Expired bool
}

// State reads one row's state, or ErrNotFound when the row is gone. It locks the
// row, so a hold and an expiry decided on the same row cannot both read it as
// free.
func (r Registry) State(ctx context.Context, tx pgx.Tx, t privacy.Table, id ids.UUID) (RowState, error) {
	var s RowState
	err := tx.QueryRow(ctx, `SELECT
		EXISTS (SELECT 1 FROM extension_legal_hold h WHERE h.table_name = $2 AND h.row_id = t.id),
		EXISTS (SELECT 1 FROM extension_expired_row x WHERE x.table_name = $2 AND x.row_id = t.id)
		FROM `+qualified(t.Name)+` t WHERE t.id = $1 FOR UPDATE OF t`, id, t.Name).Scan(&s.Held, &s.Expired)
	if errors.Is(err, pgx.ErrNoRows) {
		return RowState{}, apperrors.ErrNotFound
	}
	if err != nil {
		return RowState{}, fmt.Errorf("extprivacy: reading a row of %s: %w", t.Name, err)
	}
	return s, nil
}

// SetHeld places or lifts the hold on one row. A row that is gone is not found.
// A hold already in the state asked for is a conflict, so a no-op writes no
// audit row claiming a change.
func (r Registry) SetHeld(ctx context.Context, tx pgx.Tx, t privacy.Table, id ids.UUID, held bool) error {
	if !identifiable(t) {
		return fmt.Errorf("extprivacy: %s declares no personal column and no age column, so its rows have no id to hold: %w", t.Name, apperrors.ErrInvalidArgument)
	}
	state, err := r.State(ctx, tx, t, id)
	if err != nil {
		return err
	}
	if state.Held == held {
		return apperrors.ErrConflict
	}
	query := `INSERT INTO extension_legal_hold (table_name, row_id) VALUES ($1, $2)`
	if !held {
		query = `DELETE FROM extension_legal_hold WHERE table_name = $1 AND row_id = $2`
	}
	if _, err := tx.Exec(ctx, query, t.Name, id); err != nil {
		return fmt.Errorf("extprivacy: writing the hold on a row of %s: %w", t.Name, err)
	}
	return nil
}

// identifiable reports whether the core can name the rows of a table by id.
// The boot check requires a uuid id of a table with personal columns or an age
// column. Holds and sweeps act on those tables only.
func identifiable(t privacy.Table) bool { return len(t.Personal) > 0 || t.AgeColumn != "" }

// Window bounds which rows of one table a sweep may take.
type Window struct {
	// OlderThan: only a row whose age column is before it is due.
	OlderThan time.Time
	// A floor carried from a stricter earlier rule also keeps the rows from
	// before CarriedBefore and from CarriedSince on. Both are nil without one.
	CarriedBefore, CarriedSince *time.Time
}

// dueSQL is what a window allows over the table aliased `t`, less the rows that
// name a held contact, lead or company. $1, $2 and $3 are the window's three
// instants. Due and StillDue share it.
func dueSQL(t privacy.Table) string {
	age := "t." + pgx.Identifier{t.AgeColumn}.Sanitize()
	parts := []string{age + ` < $1 AND ($2::timestamptz IS NULL OR NOT (` + age + ` < $2 AND ` + age + ` >= $3))`}
	for _, l := range t.Links {
		if l.Record == privacy.Contact || l.Record == privacy.Lead || l.Record == privacy.Company {
			parts = append(parts, `NOT EXISTS (SELECT 1 FROM `+pgx.Identifier{string(l.Record)}.Sanitize()+
				` k WHERE k.id = t.`+pgx.Identifier{l.Column}.Sanitize()+` AND k.legal_hold)`)
		}
	}
	return strings.Join(parts, " AND ")
}

// Due returns up to limit rows of a table that the window allows to expire,
// oldest first. A held row, a row naming a held record and a row already
// cleared are not due.
func (r Registry) Due(ctx context.Context, tx pgx.Tx, t privacy.Table, w Window, limit int) ([]ids.UUID, error) {
	rows, err := tx.Query(ctx, `SELECT t.id FROM `+qualified(t.Name)+` t
		WHERE `+dueSQL(t)+`
		  AND NOT EXISTS (SELECT 1 FROM extension_legal_hold h WHERE h.table_name = $4 AND h.row_id = t.id)
		  AND NOT EXISTS (SELECT 1 FROM extension_expired_row x WHERE x.table_name = $4 AND x.row_id = t.id)
		ORDER BY t.`+pgx.Identifier{t.AgeColumn}.Sanitize()+`, t.id LIMIT $5`,
		w.OlderThan, w.CarriedBefore, w.CarriedSince, t.Name, limit)
	if err != nil {
		return nil, fmt.Errorf("extprivacy: selecting the rows of %s past their window: %w", t.Name, err)
	}
	return pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
}

// StillDue asks again about one row, with the predicate Due selects by.
func (r Registry) StillDue(ctx context.Context, tx pgx.Tx, t privacy.Table, w Window, id ids.UUID) (bool, error) {
	var due bool
	err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM `+qualified(t.Name)+` t WHERE t.id = $4 AND `+dueSQL(t)+`)`,
		w.OlderThan, w.CarriedBefore, w.CarriedSince, id).Scan(&due)
	if err != nil {
		return false, fmt.Errorf("extprivacy: checking a row of %s against its window: %w", t.Name, err)
	}
	return due, nil
}

// Expire ends the retention of one row. A table with no personal column loses
// the row. Any other keeps it with its personal columns cleared and marked, so
// no later pass selects it again.
func (r Registry) Expire(ctx context.Context, tx pgx.Tx, t privacy.Table, id ids.UUID) error {
	if len(t.Personal) == 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM `+qualified(t.Name)+` WHERE id = $1`, id); err != nil {
			return fmt.Errorf("extprivacy: deleting a row of %s: %w", t.Name, err)
		}
		return nil
	}
	set, err := clearAssignments(ctx, tx, t)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "UPDATE "+qualified(t.Name)+" SET "+set+" WHERE id = $1", id); err != nil {
		return fmt.Errorf("extprivacy: clearing a row of %s: %w", t.Name, err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO extension_expired_row (table_name, row_id) VALUES ($1, $2)
		ON CONFLICT DO NOTHING`, t.Name, id); err != nil {
		return fmt.Errorf("extprivacy: marking a cleared row of %s: %w", t.Name, err)
	}
	return nil
}
