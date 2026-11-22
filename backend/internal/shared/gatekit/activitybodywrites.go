// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gatekit

import (
	"regexp"
	"slices"
	"strings"
)

// The three clauses that can assign an activity's columns.
var (
	insertsActivity = regexp.MustCompile(`(?s)\binsert\s+into\s+(?:"?[a-z_][a-z_0-9]*"?\s*\.\s*)?"?activity"?(?:\s+as\s+[a-z_][a-z_0-9]*)?\s*\(`)
	upsertSet       = regexp.MustCompile(`(?s)\bdo\s+update\s+set\s`)
	setKeyword      = regexp.MustCompile(`(?s)^(?:\s*(?:as\s+)?[a-z_][a-z_0-9]*)?\s*\bset\s`)
	setClauseEnd    = regexp.MustCompile(`^(?:where|from|returning)\b`)
)

// ActivityBodyWrites counts the clauses of a SQL statement that assign an
// activity's body, and those among them that leave body_own_end unassigned.
// The stored end is read with the body, so a writer of one sets the other.
func ActivityBodyWrites(statement string) (writers, unpaired int) {
	low := strings.ToLower(statement)
	var clauses [][]string
	for _, at := range insertsActivity.FindAllStringIndex(low, -1) {
		clauses = append(clauses, bareColumns(sqlListAt(low, at[1], false)))
		if set := upsertSet.FindStringIndex(low[at[1]:]); set != nil {
			clauses = append(clauses, assignedColumns(sqlListAt(low, at[1]+set[1], true)))
		}
	}
	for _, at := range updatesActivity.FindAllStringIndex(low, -1) {
		if set := setKeyword.FindStringIndex(low[at[1]:]); set != nil {
			clauses = append(clauses, assignedColumns(sqlListAt(low, at[1]+set[1], true)))
		}
	}
	for _, columns := range clauses {
		if !slices.Contains(columns, "body") {
			continue
		}
		writers++
		if !slices.Contains(columns, "body_own_end") {
			unpaired++
		}
	}
	return writers, unpaired
}

// sqlListAt splits the comma list starting at pos into its items. It stops at
// the parenthesis that closes it, or at the word that ends a SET clause.
func sqlListAt(text string, pos int, setClause bool) []string {
	var items []string
	depth, start := 0, pos
	for i := pos; i < len(text); i++ {
		if span, _ := SQLSpanAt(text, i); span > 0 {
			i += span - 1
			continue
		}
		atWord := i == pos || !identStart(text[i-1]) && text[i-1] != '.'
		switch {
		case text[i] == '(':
			depth++
		case text[i] == ')' && depth == 0, setClause && depth == 0 && atWord && setClauseEnd.MatchString(text[i:]):
			return append(items, text[start:i])
		case text[i] == ')':
			depth--
		case text[i] == ',' && depth == 0:
			items = append(items, text[start:i])
			start = i + 1
		}
	}
	return append(items, text[start:])
}

// assignedColumns names the columns each assignment of a SET clause writes. A
// target in parentheses, as in (body, subject) = (...), names several.
func assignedColumns(assignments []string) []string {
	columns := make([]string, 0, len(assignments))
	for _, assignment := range assignments {
		target, _, _ := strings.Cut(assignment, "=")
		target = strings.TrimSpace(target)
		if strings.HasPrefix(target, "(") {
			columns = append(columns, strings.Split(strings.Trim(target, "()"), ",")...)
			continue
		}
		columns = append(columns, target)
	}
	return bareColumns(columns)
}

// bareColumns strips the alias, the quotes and the comments from column names.
func bareColumns(columns []string) []string {
	bare := make([]string, 0, len(columns))
	for _, column := range columns {
		fields := strings.Fields(sqlComment.ReplaceAllString(column, " "))
		if len(fields) == 0 {
			continue
		}
		name := fields[len(fields)-1]
		bare = append(bare, strings.Trim(name[strings.LastIndexByte(name, '.')+1:], `"`))
	}
	return bare
}

// sqlComment is a line comment or a block comment, which SQL reads as a space.
var sqlComment = regexp.MustCompile(`(?s)--[^\n]*|/\*.*?\*/`)
