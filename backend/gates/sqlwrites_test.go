// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// Reading a SQL literal for what it WRITES.
//
// Its own file because it is not one gate's helper: table ownership, the PII
// censuses, the satellite lifecycle, the person scrub and the pending-writer
// claim all ask it, and the last time two of them read statements separately
// the two answers drifted.
//
// Everything here is a FLOOR and never a ceiling. A statement assembled at
// runtime, an INSERT … SELECT with no column list, a fragment naming no table
// — each contributes nothing, so a caller must read an empty answer as
// "unknown" and never as "writes nothing". Every gate over this corpus carries
// its own floor assertion for that reason.

import (
	"regexp"
	"strings"
)

// sqlWriteTargets extracts write-statement table names from one SQL (or
// SQL-carrying format) string. UPDATE requires a SET clause so prose and
// `DO UPDATE SET`/`FOR UPDATE` never match; INSERT/DELETE are unambiguous.
var (
	insertRe = regexp.MustCompile(`(?is)\binsert\s+into\s+([a-z_][a-z0-9_]*)`)
	deleteRe = regexp.MustCompile(`(?is)\bdelete\s+from\s+([a-z_][a-z0-9_]*)`)
	updateRe = regexp.MustCompile(`(?is)\b(do\s+|for\s+)?update\s+([a-z_][a-z0-9_]*)\s+(?:[a-z_][a-z0-9_]*\s+)?set\b`)
)

var (
	insertColsRe = regexp.MustCompile(`(?is)\binsert\s+into\s+[a-z_][a-z0-9_]*\s*\(([^)]*)\)`)
	setColRe     = regexp.MustCompile(`(?is)([a-z_][a-z0-9_]*)\s*=`)
)

// insertColumns reads the column list of the FIRST insert in a literal. One
// statement per literal is this tree's style; a literal carrying two inserts
// gets the first one's list, which is why cols is a floor.
func insertColumns(literal string) []string {
	m := insertColsRe.FindStringSubmatch(literal)
	if m == nil {
		return nil
	}
	var cols []string
	for _, raw := range strings.Split(m[1], ",") {
		name := strings.ToLower(strings.Trim(strings.TrimSpace(raw), `"`))
		if name != "" {
			cols = append(cols, name)
		}
	}
	return cols
}

// setColumns reads the assignment targets of an UPDATE's SET clause, stopping
// at WHERE so a predicate's comparisons are not read as writes.
func setColumns(literal string) []string {
	lower := strings.ToLower(literal)
	at := strings.Index(lower, " set ")
	if at < 0 {
		return nil
	}
	clause := literal[at+len(" set "):]
	if end := strings.Index(strings.ToLower(clause), " where "); end >= 0 {
		clause = clause[:end]
	}
	var cols []string
	for _, m := range setColRe.FindAllStringSubmatch(clause, -1) {
		cols = append(cols, strings.ToLower(m[1]))
	}
	return cols
}

// sqlWriteTargets is sqlWrites projected to the table names, which is all most
// callers ask. One scan underneath, so a statement shape learned by one view
// is learned by both.
func sqlWriteTargets(literal string) []string {
	writes := sqlWrites(literal)
	tables := make([]string, 0, len(writes))
	for _, w := range writes {
		tables = append(tables, w.table)
	}
	return tables
}

func sqlWrites(literal string) []sqlTarget {
	var tables []sqlTarget
	for _, m := range insertRe.FindAllStringSubmatch(literal, -1) {
		tables = append(tables, sqlTarget{strings.ToLower(m[1]), "insert", insertColumns(literal)})
	}
	for _, m := range deleteRe.FindAllStringSubmatch(literal, -1) {
		tables = append(tables, sqlTarget{strings.ToLower(m[1]), "delete", nil})
	}
	for _, m := range updateRe.FindAllStringSubmatch(literal, -1) {
		if m[1] != "" { // ON CONFLICT … DO UPDATE / SELECT … FOR UPDATE — not a new target
			continue
		}
		tables = append(tables, sqlTarget{strings.ToLower(m[2]), "update", setColumns(literal)})
	}
	return tables
}
