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
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"sync"

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

// installed is this process's set. Sealed at boot by the composition root and
// read on every merge and erasure.
var installed struct {
	mu     sync.RWMutex
	sealed bool
	tables []Owned
}

// ErrNotSealed refuses a merge or an erasure in a process that never installed
// the set. Running with an empty one would leave declared rows naming a retired
// record, or holding an erased subject, and report success.
var ErrNotSealed = errors.New("sweepreach: the declared tables were never installed in this process, so the merge or erasure would miss them; install them at boot (compose.RegisterExtensions)")

// Install validates the whole set and then replaces the installed one. A set
// with one bad entry installs none, so a refused boot leaves nothing half
// reached.
func Install(set []Owned) error {
	seen := make(map[string]string, len(set))
	for _, o := range set {
		if err := o.Table.Validate(); err != nil {
			return fmt.Errorf("sweepreach: %s: %w", o.Owner, err)
		}
		if o.Schema != "" && o.Schema != "ext" {
			return fmt.Errorf("sweepreach: %s declares table %q in schema %q; a table is core or ext", o.Owner, o.Table.Name, o.Schema)
		}
		if prior, dup := seen[o.Table.Name]; dup {
			return fmt.Errorf("sweepreach: table %q is declared by both %s and %s", o.Table.Name, prior, o.Owner)
		}
		seen[o.Table.Name] = o.Owner
	}
	sorted := slices.Clone(set)
	slices.SortFunc(sorted, func(a, b Owned) int { return cmp.Compare(a.Table.Name, b.Table.Name) })
	installed.mu.Lock()
	defer installed.mu.Unlock()
	installed.tables, installed.sealed = sorted, true
	return nil
}

// InstallForTestDatabase seals an empty set when nothing is installed yet. A
// test database calls it, so a module's own suite merges with no declared
// tables. It never replaces an installed set.
func InstallForTestDatabase() {
	installed.mu.Lock()
	defer installed.mu.Unlock()
	installed.sealed = true
}

// Tables returns the installed set, ordered by table name, and whether one was
// installed at all.
func Tables() ([]Owned, error) {
	installed.mu.RLock()
	defer installed.mu.RUnlock()
	if !installed.sealed {
		return nil, ErrNotSealed
	}
	return slices.Clone(installed.tables), nil
}

func (o Owned) qualified() string {
	if o.Schema == "" {
		return pgx.Identifier{o.Table.Name}.Sanitize()
	}
	return pgx.Identifier{o.Schema, o.Table.Name}.Sanitize()
}

// Relink moves every declared column holding the retired record's id onto the
// survivor, in the caller's transaction. A plain UPDATE is right because a
// declared link is a pointer and nothing makes it unique per record.
func Relink(ctx context.Context, tx pgx.Tx, record reach.Record, source, target ids.UUID) error {
	tables, err := Tables()
	if err != nil {
		return err
	}
	for _, o := range tables {
		for _, l := range o.Table.Links {
			column := pgx.Identifier{l.Column}.Sanitize()
			stmt := `UPDATE ` + o.qualified() + ` SET ` + column + ` = $2 WHERE ` + column + ` = $1`
			args := []any{source, target}
			switch {
			case l.KindColumn != "":
				stmt += ` AND ` + pgx.Identifier{l.KindColumn}.Sanitize() + ` = $3`
				args = append(args, string(record))
			case l.Record != record:
				continue
			}
			if _, err := tx.Exec(ctx, stmt, args...); err != nil {
				return fmt.Errorf("move %s's %s.%s onto the surviving %s: %w", o.Owner, o.Table.Name, l.Column, record, err)
			}
		}
	}
	return nil
}

// Cleared is one table the erasure wrote and the rows it cleared there.
type Cleared struct {
	Table string
	Rows  []ids.UUID
}

// Erase clears the declared columns on every row a contact link names the
// subject in, in the caller's transaction. tombstone is the text a Tombstone
// column receives. The caller writes the audit row for each cleared row.
func Erase(ctx context.Context, tx pgx.Tx, subject ids.UUID, tombstone string) ([]Cleared, error) {
	tables, err := Tables()
	if err != nil {
		return nil, err
	}
	var out []Cleared
	for _, o := range tables {
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
		out = append(out, Cleared{Table: o.Table.Name, Rows: cleared})
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
