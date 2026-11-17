// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package sweepreach holds the tables the core's merge and Art. 17 erasure
// reach because their owner declared them (pkg/extension/reach). The owner is
// a core module or an extension unit. The merge and the erasure run the SQL
// here, so neither names the owner's tables in its own source.
package sweepreach

import (
	"cmp"
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// Owned is one declared table and who declared it. Schema is empty for a core
// table and "ext" for a unit's.
type Owned struct {
	Owner  string
	Schema string
	Table  reach.Table
}

// Set is one composition's declared tables, ordered by table name. The
// composition builds it at boot and hands it to every merge and erasure door.
// So two compositions in one process never read each other's tables. The zero
// Set declares no table.
type Set struct {
	tables []Owned
}

// NewSet validates the whole set. A set with one bad entry is refused whole, so
// a refused boot reaches nothing.
func NewSet(owned []Owned) (Set, error) {
	seen := make(map[string]string, len(owned))
	for _, o := range owned {
		if err := o.Table.Validate(); err != nil {
			return Set{}, fmt.Errorf("sweepreach: %s: %w", o.Owner, err)
		}
		if o.Schema != "" && o.Schema != "ext" {
			return Set{}, fmt.Errorf("sweepreach: %s declares table %q in schema %q; a table is core or ext", o.Owner, o.Table.Name, o.Schema)
		}
		if prior, dup := seen[o.Table.Name]; dup {
			return Set{}, fmt.Errorf("sweepreach: table %q is declared by both %s and %s", o.Table.Name, prior, o.Owner)
		}
		seen[o.Table.Name] = o.Owner
	}
	sorted := slices.Clone(owned)
	slices.SortFunc(sorted, func(a, b Owned) int { return cmp.Compare(a.Table.Name, b.Table.Name) })
	return Set{tables: sorted}, nil
}

// Tables returns the set, ordered by table name.
func (s Set) Tables() []Owned { return slices.Clone(s.tables) }

// entity is the audit entity a row of the table is recorded under.
func (o Owned) entity() string { return cmp.Or(o.Table.Entity, o.Table.Name) }

func (o Owned) qualified() string {
	if o.Schema == "" {
		return pgx.Identifier{o.Table.Name}.Sanitize()
	}
	return pgx.Identifier{o.Schema, o.Table.Name}.Sanitize()
}

// Relinked is one declared column the merge moved, the audit entity of its
// table, and the rows it moved there.
type Relinked struct {
	Entity, Column string
	Rows           []ids.UUID
}

// Relink moves every declared column holding the retired record's id onto the
// survivor, in the caller's transaction. A plain UPDATE is right because a
// declared link is a pointer and nothing makes it unique per record. The
// caller writes the audit row for each moved row, as for each row Erase clears.
func (s Set) Relink(ctx context.Context, tx pgx.Tx, record reach.Record, source, target ids.UUID) ([]Relinked, error) {
	var out []Relinked
	for _, o := range s.tables {
		for _, l := range o.Table.Links {
			if !movesFor(l, record) {
				continue
			}
			column := pgx.Identifier{l.Column}.Sanitize()
			stmt := `UPDATE ` + o.qualified() + ` SET ` + column + ` = $2 WHERE ` + column + ` = $1`
			args := []any{source, target}
			if l.KindColumn != "" {
				stmt += ` AND ` + pgx.Identifier{l.KindColumn}.Sanitize() + ` = $3`
				args = append(args, string(record))
			}
			rows, err := tx.Query(ctx, stmt+` RETURNING id`, args...)
			if err != nil {
				return nil, fmt.Errorf("move %s's %s.%s onto the surviving %s: %w", o.Owner, o.Table.Name, l.Column, record, err)
			}
			moved, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
			if err != nil {
				return nil, fmt.Errorf("move %s's %s.%s onto the surviving %s: %w", o.Owner, o.Table.Name, l.Column, record, err)
			}
			if len(moved) > 0 {
				out = append(out, Relinked{Entity: o.entity(), Column: l.Column, Rows: moved})
			}
		}
	}
	return out, nil
}

// movesFor reports whether Relink moves any row of the link when a record of
// this kind is merged away. A kind-column link moves only the rows whose kind
// column names that kind.
func movesFor(l reach.Link, record reach.Record) bool {
	return l.KindColumn != "" || l.Record == record
}

// Column names one declared link column. Schema is empty for a core table.
type Column struct {
	Schema, Table, Name string
}

// Moved says which rows of one column Relink moves. With KindColumn empty, it
// moves every row naming the retired record. Otherwise it moves only the rows
// whose KindColumn holds the merged kind.
type Moved struct {
	KindColumn string
}

// MovedColumns returns the declared columns Relink moves when a record of this
// kind is merged away, and which of its rows. The merge's dependents check
// reads it. A row the merge carries onto the survivor never blocks that merge,
// and a row it leaves behind still does.
func (s Set) MovedColumns(record reach.Record) (map[Column]Moved, error) {
	if err := record.Validate(); err != nil {
		return nil, err
	}
	moved := map[Column]Moved{}
	for _, o := range s.tables {
		for _, l := range o.Table.Links {
			if movesFor(l, record) {
				moved[Column{Schema: o.Schema, Table: o.Table.Name, Name: l.Column}] = Moved{KindColumn: l.KindColumn}
			}
		}
	}
	return moved, nil
}

// Cleared is the audit entity of one table the erasure wrote, and the rows it
// cleared there.
type Cleared struct {
	Entity string
	Rows   []ids.UUID
}

// Erase clears the declared columns on every row a contact link names the
// subject in, in the caller's transaction. tombstone is the text a Tombstone
// column receives. The caller writes the audit row for each cleared row.
func (s Set) Erase(ctx context.Context, tx pgx.Tx, subject ids.UUID, tombstone string) ([]Cleared, error) {
	var out []Cleared
	for _, o := range s.tables {
		if len(o.Table.Erase) == 0 {
			continue
		}
		stmt, args := eraseStatement(o, subject, tombstone)
		rows, err := tx.Query(ctx, stmt, args...)
		if err != nil {
			return nil, fmt.Errorf("clear %s's %s rows naming the subject: %w", o.Owner, o.Table.Name, err)
		}
		cleared, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
		if err != nil {
			return nil, fmt.Errorf("read %s's cleared %s rows: %w", o.Owner, o.Table.Name, err)
		}
		out = append(out, Cleared{Entity: o.entity(), Rows: cleared})
	}
	return out, nil
}

// eraseStatement builds one UPDATE … RETURNING id. Placeholders follow the
// argument list: $1 is the subject, and the tombstone takes the next one only
// when a column writes it.
func eraseStatement(o Owned, subject ids.UUID, tombstone string) (string, []any) {
	args := []any{subject}
	sets := make([]string, 0, len(o.Table.Erase))
	for _, c := range o.Table.Erase {
		value := "NULL"
		switch c.To {
		case reach.Empty:
			value = "''"
		case reach.Tombstone:
			if len(args) == 1 {
				args = append(args, tombstone)
			}
			value = "$" + strconv.Itoa(len(args))
		}
		sets = append(sets, pgx.Identifier{c.Column}.Sanitize()+" = "+value)
	}
	var names []string
	for _, l := range o.Table.Links {
		column := pgx.Identifier{l.Column}.Sanitize() + " = $1"
		switch {
		case l.KindColumn != "":
			names = append(names, "("+pgx.Identifier{l.KindColumn}.Sanitize()+" = '"+string(reach.Contact)+"' AND "+column+")")
		case l.Record == reach.Contact:
			names = append(names, column)
		}
	}
	return `UPDATE ` + o.qualified() + ` SET ` + strings.Join(sets, ", ") +
		` WHERE ` + strings.Join(names, " OR ") + ` RETURNING id`, args
}

// EraseSQL is the statement Erase runs for one declared table, without its
// arguments. The privacy gates read it so they check the SQL that runs.
func EraseSQL(o Owned) string {
	stmt, _ := eraseStatement(o, ids.UUID{}, "")
	return stmt
}
