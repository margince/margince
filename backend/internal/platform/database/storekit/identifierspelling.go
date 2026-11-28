// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// IdentifierColumn is supplied by the owning module. Parent is the UUID column
// whose record owns the value; configuration rows use their own id.
type IdentifierColumn struct {
	Table, Column, Object, Parent string
	// HashedDomainKey pages a text primary key without putting it in progress.
	HashedDomainKey bool
}

// IdentifierPage includes unchanged rows so each bounded pass advances.
type IdentifierPage struct {
	Scanned []ids.UUID
	Changed int
	Review  []ids.UUID
}

type identifierRow struct {
	ID, Parent ids.UUID
	Value      *string
}

func (c IdentifierColumn) validate() error {
	if c.HashedDomainKey && (c.Column != "domain" || c.Parent != "") {
		return errors.New("identifier text key needs a domain without a UUID parent")
	}
	if c.Table == "" || c.Object == "" || (c.Parent == "" && !c.HashedDomainKey) {
		return errors.New("identifier repair needs its owning table and record")
	}
	switch c.Column {
	case "email", "address", "domain":
		return nil
	default:
		return errors.New("identifier repair names no address or domain column")
	}
}

// Repair canonicalizes a bounded page. A conflicting value stays untouched for
// operator review; it never chooses which user's or contact's identity to keep.
func (c IdentifierColumn) Repair(ctx context.Context, tx pgx.Tx, after ids.UUID, limit int) (IdentifierPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return IdentifierPage{}, err
	}
	if err := c.validate(); err != nil {
		return IdentifierPage{}, err
	}
	if limit < 1 {
		return IdentifierPage{}, errors.New("identifier repair needs a positive batch limit")
	}
	var args []any
	arg := func(value any) string { args = append(args, value); return "$" + strconv.Itoa(len(args)) }
	parent := pgx.Identifier{c.Parent}.Sanitize()
	if c.HashedDomainKey {
		id := MustWorkspace(ctx)
		if id.IsZero() {
			return IdentifierPage{}, errors.New("identifier repair needs the installation identity")
		}
		parent = arg(id) + "::uuid"
	}
	key := c.keySQL()
	query := "SELECT " + key + ", " + parent + ", " + pgx.Identifier{c.Column}.Sanitize() + " FROM " + pgx.Identifier{c.Table}.Sanitize() + " WHERE " + key + " > " + arg(after) + " ORDER BY " + key + " LIMIT " + arg(limit) + " FOR UPDATE"
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return IdentifierPage{}, err
	}
	page, err := pgx.CollectRows(rows, pgx.RowToStructByPos[identifierRow])
	if err != nil {
		return IdentifierPage{}, err
	}
	out := IdentifierPage{}
	for _, row := range page {
		out.Scanned = append(out.Scanned, row.ID)
		if row.Value == nil {
			continue
		}
		folded := correspondence.Fold(*row.Value)
		if c.Column == "domain" {
			folded = owndomain.Fold(*row.Value)
		}
		if folded == *row.Value {
			continue
		}
		if folded == "" {
			out.Review = append(out.Review, row.Parent)
			continue
		}
		changed, review, err := c.repairRow(ctx, tx, row, folded)
		if err != nil {
			return IdentifierPage{}, err
		}
		if changed {
			out.Changed++
		}
		if review {
			out.Review = append(out.Review, row.Parent)
		}
	}
	return out, nil
}

func (c IdentifierColumn) repairRow(ctx context.Context, tx pgx.Tx, row identifierRow, value string) (bool, bool, error) {
	child, err := tx.Begin(ctx)
	if err != nil {
		return false, false, err
	}
	err = c.writeRow(ctx, child, row, value)
	if err != nil {
		if rollback := child.Rollback(ctx); rollback != nil {
			return false, false, fmt.Errorf("identifier repair rollback: %w", rollback)
		}
		var pg *pgconn.PgError
		if errors.As(err, &pg) && (pg.Code == "23505" || pg.Code == "23514") {
			return false, true, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			return false, false, nil
		}
		return false, false, err
	}
	if err := child.Commit(ctx); err != nil {
		return false, false, err
	}
	return true, false, nil
}

func (c IdentifierColumn) writeRow(ctx context.Context, tx pgx.Tx, row identifierRow, value string) error {
	args := []any{}
	arg := func(value any) string { args = append(args, value); return "$" + strconv.Itoa(len(args)) }
	set := pgx.Identifier{c.Column}.Sanitize() + " = " + arg(value)
	if domain, ok := FoldedDomainOf(c.Table); ok && domain.Source == c.Column {
		set += ", " + pgx.Identifier{domain.Folded}.Sanitize() + " = " + arg(domain.Fold(value))
	}
	where := c.keySQL() + " = " + arg(row.ID)
	if c.HashedDomainKey {
		where += " AND " + pgx.Identifier{c.Column}.Sanitize() + " = " + arg(*row.Value)
	}
	query := "UPDATE " + pgx.Identifier{c.Table}.Sanitize() + " SET " + set + " WHERE " + where + " RETURNING " + pgx.Identifier{c.Column}.Sanitize()
	var written string
	if err := tx.QueryRow(ctx, query, args...).Scan(&written); err != nil {
		return err
	}
	audit, err := Audit(ctx, tx, "update", c.Object, row.Parent, identifierImage(c.Column, *row.Value), identifierImage(c.Column, written))
	if err != nil {
		return err
	}
	return EmitEventForEntity(ctx, tx, audit, c.Object, row.Parent, crmcontracts.InternalEventIdentifierSpellingChanged{Column: c.Column})
}

// Count inspects at most maximum+1 primary keys, including unchanged rows.
func (c IdentifierColumn) Count(ctx context.Context, q interface {
	QueryRow(context.Context, string, ...any) pgx.Row
}, maximum int64) (int64, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, err
	}
	if err := c.validate(); err != nil {
		return 0, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT "+c.keySQL()+" FROM "+pgx.Identifier{c.Table}.Sanitize()+" LIMIT @maximum) inspected", pgx.StrictNamedArgs{"maximum": maximum + 1}).Scan(&count)
	return count, err
}

//craft:ignore naked-any the audit images contain only a fixed identifier field
func identifierImage(column, value string) map[string]any {
	switch column {
	case "email":
		return map[string]any{"email": value}
	case "address":
		return map[string]any{"address": value}
	case "domain":
		return map[string]any{"domain": value}
	default:
		return nil
	}
}

func (c IdentifierColumn) keySQL() string {
	if c.HashedDomainKey {
		return "identifier_spelling_row_id(" + pgx.Identifier{c.Column}.Sanitize() + ")"
	}
	return "id"
}
