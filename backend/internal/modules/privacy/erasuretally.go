// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// The erasure's count of its own work. Every arm of the cascade runs on a
// tallyTx, which records the rows each writing statement touched. A new arm is
// counted because it exists, not because somebody remembered to count it.

import (
	"context"
	"maps"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// tallyTx is a transaction that counts the rows its writing statements touch,
// by "<table>.<verb>". It counts no savepoint: no arm of the cascade opens one,
// and TestTheErasureCascadeOpensNoSavepoint fails the arm that does.
type tallyTx struct {
	pgx.Tx
	rows map[string]int64
}

func newTallyTx(tx pgx.Tx) *tallyTx { return &tallyTx{Tx: tx, rows: map[string]int64{}} }

// mutationTarget finds the first write a statement makes. A row lock (FOR
// UPDATE) and an upsert's DO UPDATE are not a target of their own.
var (
	mutationTarget = regexp.MustCompile(`(?is)\b(UPDATE|DELETE\s+FROM|INSERT\s+INTO)\s+(?:ONLY\s+)?"?([a-z_][a-z0-9_]*)`)
	notATarget     = regexp.MustCompile(`(?is)\bFOR\s+(?:NO\s+KEY\s+)?UPDATE\b|\bDO\s+UPDATE\b`)
	verbOf         = map[string]string{"UPDATE": "updated", "DELETE": "deleted", "INSERT": "inserted"}
)

// bookkeeping are the tables a write lands in to record the erasure rather
// than to erase anything.
var bookkeeping = map[string]bool{storekit.TableAudit: true, "event_outbox": true}

// categoryOf answers the category a statement's rows count under, or false
// for a statement that writes nothing the account reports.
func categoryOf(sql string) (string, bool) {
	match := mutationTarget.FindStringSubmatch(notATarget.ReplaceAllString(sql, ""))
	if match == nil || bookkeeping[match[2]] {
		return "", false
	}
	verb := verbOf[strings.ToUpper(strings.Fields(match[1])[0])]
	return match[2] + "." + verb, true
}

func (t *tallyTx) note(sql string, rows int64) {
	if category, counted := categoryOf(sql); counted && rows > 0 {
		t.rows[category] += rows
	}
}

func (t *tallyTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tag, err := t.Tx.Exec(ctx, sql, args...)
	if err == nil {
		t.note(sql, tag.RowsAffected())
	}
	return tag, err
}

// Query counts a mutating statement's returned rows, one per row read.
//
//nolint:ireturn // pgx.Tx's own signature.
func (t *tallyTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	rows, err := t.Tx.Query(ctx, sql, args...)
	if err != nil {
		return rows, err
	}
	if _, counted := categoryOf(sql); !counted {
		return rows, nil
	}
	return &tallyRows{Rows: rows, tx: t, sql: sql}, nil
}

//nolint:ireturn // pgx.Tx's own signature.
func (t *tallyTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	row := t.Tx.QueryRow(ctx, sql, args...)
	if _, counted := categoryOf(sql); !counted {
		return row
	}
	return tallyRow{row: row, tx: t, sql: sql}
}

func (t *tallyTx) CopyFrom(ctx context.Context, table pgx.Identifier, columns []string, source pgx.CopyFromSource) (int64, error) {
	copied, err := t.Tx.CopyFrom(ctx, table, columns, source)
	if err == nil && len(table) > 0 {
		t.note("INSERT INTO "+table[len(table)-1], copied)
	}
	return copied, err
}

// counted answers a copy of the counts so far.
func (t *tallyTx) counted() map[string]int64 {
	return maps.Clone(t.rows)
}

type tallyRows struct {
	pgx.Rows
	tx  *tallyTx
	sql string
}

func (r *tallyRows) Next() bool {
	more := r.Rows.Next()
	if more {
		r.tx.note(r.sql, 1)
	}
	return more
}

type tallyRow struct {
	row pgx.Row
	tx  *tallyTx
	sql string
}

func (r tallyRow) Scan(dest ...any) error {
	err := r.row.Scan(dest...)
	if err == nil {
		r.tx.note(r.sql, 1)
	}
	return err
}
