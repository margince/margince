// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package reach lets a unit declare which columns of its own tables hold the
// id of a core company, contact or lead. The core's company and contact merge
// moves those columns onto the survivor, and the core's Art. 17 erasure clears
// the columns a unit names. The unit never runs either sweep itself, and core
// never names the unit's tables in its own SQL.
//
// These types evolve additively or through versioned successors, never in
// place (EXT-P3).
//
//margince:extension-surface
package reach

import (
	"fmt"
	"regexp"
)

// Record is the kind of core record a column holds the id of. Closed: the
// merge and the erasure only know these three.
type Record string

// The record kinds a column can hold.
const (
	Company Record = "company"
	Contact Record = "contact"
	Lead    Record = "lead"
)

// Validate enforces membership in the closed set.
func (r Record) Validate() error {
	switch r {
	case Company, Contact, Lead:
		return nil
	}
	return fmt.Errorf("record kind %q is not one the merge moves (company, contact, lead)", string(r))
}

// Link says one column holds the id of a core record.
//
// Set Record when the column always holds one kind, such as a company id. Set
// KindColumn instead when a second column names the kind per row, such as a
// (linked_record_type, linked_record_id) pair. Then the merge moves the rows
// whose kind is the kind being merged, and the erasure finds the rows whose
// kind is contact.
type Link struct {
	Column     string
	Record     Record
	KindColumn string
}

// Value is what the erasure writes into a cleared column.
type Value string

// The values an erasure can write.
const (
	// Null clears the column to NULL.
	Null Value = "null"
	// Empty clears a text column to the empty string.
	Empty Value = "empty"
	// Tombstone writes the core's fixed text saying a named contact was erased.
	// It is for a text column whose constraint refuses an empty value.
	Tombstone Value = "tombstone"
)

// Clear says the erasure writes one column with one value.
type Clear struct {
	Column string
	To     Value
}

// Table is the declaration for one table.
type Table struct {
	// Name is the table's unqualified name. A unit's table is its complete
	// `ext_<namespace>_<table>` name.
	Name string

	// Links are the columns holding a core record's id. The merge moves each
	// one onto the survivor.
	Links []Link

	// Erase are the columns the erasure clears on every row a contact link
	// names the subject in. List every column a CHECK constraint ties to
	// another, or the statement fails and the whole erasure rolls back. A
	// table with Erase must have a uuid `id` column: the core writes an audit
	// row for each row it clears.
	Erase []Clear
}

var identifier = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Validate checks the names are plain identifiers and the declaration can be
// acted on.
func (t Table) Validate() error {
	if !identifier.MatchString(t.Name) || len(t.Name) > 63 {
		return fmt.Errorf("reach table %q is not an identifier", t.Name)
	}
	if len(t.Links) == 0 {
		return fmt.Errorf("reach table %q declares no link, so neither the merge nor the erasure can find its rows", t.Name)
	}
	contact := false
	for _, l := range t.Links {
		if err := l.validate(t.Name); err != nil {
			return err
		}
		contact = contact || l.Record == Contact || l.KindColumn != ""
	}
	if len(t.Erase) > 0 && !contact {
		return fmt.Errorf("reach table %q clears columns on erasure but no link can hold a contact", t.Name)
	}
	return t.validateErase()
}

func (t Table) validateErase() error {
	seen := map[string]bool{}
	for _, c := range t.Erase {
		if !identifier.MatchString(c.Column) || c.Column == "id" {
			return fmt.Errorf("reach table %q: erase column %q is not an identifier the erasure may clear", t.Name, c.Column)
		}
		if seen[c.Column] {
			return fmt.Errorf("reach table %q clears column %q twice", t.Name, c.Column)
		}
		seen[c.Column] = true
		switch c.To {
		case Null, Empty, Tombstone:
		default:
			return fmt.Errorf("reach table %q: erase value %q is not null, empty or tombstone", t.Name, string(c.To))
		}
	}
	return nil
}

func (l Link) validate(table string) error {
	if !identifier.MatchString(l.Column) || l.Column == "id" {
		return fmt.Errorf("reach table %q: link column %q is not an identifier", table, l.Column)
	}
	switch {
	case l.KindColumn == "" && l.Record == "":
		return fmt.Errorf("reach table %q: link column %q names no record kind and no kind column", table, l.Column)
	case l.KindColumn != "" && l.Record != "":
		return fmt.Errorf("reach table %q: link column %q names both a record kind and a kind column", table, l.Column)
	case l.KindColumn != "" && (!identifier.MatchString(l.KindColumn) || l.KindColumn == l.Column):
		return fmt.Errorf("reach table %q: kind column %q is not an identifier", table, l.KindColumn)
	case l.Record != "":
		if err := l.Record.Validate(); err != nil {
			return fmt.Errorf("reach table %q: %w", table, err)
		}
	}
	return nil
}
