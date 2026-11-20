// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

// The audit rows migrations insert, and the Go statements that insert one.

import (
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// auditColumnList reads the column list an INSERT opens with.
var auditColumnList = regexp.MustCompile(`(?s)^"?\s*\(([^)]*)\)`)

// auditInsertAnywhere finds an INSERT into audit_log, with or without a column
// list.
var auditInsertAnywhere = regexp.MustCompile(`(?is)INSERT\s+INTO\s+(?:"?public"?\.)?"?audit_log\b`)

// auditMigrationTypes reads the record types the migrations' own audit rows name,
// by the file that names them. A statement whose entity_type holds no literal is
// returned as opaque.
func auditMigrationTypes(t *testing.T) (types map[string]string, opaque []string) {
	t.Helper()
	types = map[string]string{}
	root := filepath.Join(gatekit.ModuleRoot(t), "migrations")
	err := gatekit.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".up.sql") || strings.Contains(path, "testdata") {
			return err
		}
		source, err := gatekit.ReadFile(path)
		if err != nil {
			return err
		}
		sql := sqlLineComment.ReplaceAllString(string(source), " ")
		for _, at := range auditInsertAnywhere.FindAllStringIndex(sql, -1) {
			rest := sql[at[1]:]
			if end := strings.Index(rest, ";"); end >= 0 {
				rest = rest[:end]
			}
			// No column list means the entity type's position cannot be read.
			column := -1
			if list := auditColumnList.FindStringSubmatchIndex(rest); list != nil {
				column = slices.Index(columnNames(rest[list[2]:list[3]]), "entity_type")
				rest = rest[list[1]:]
			}
			found := auditStatementTypes(rest, column)
			if len(found) == 0 {
				opaque = append(opaque, path)
			}
			for _, value := range found {
				types[value] = filepath.Base(path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("reading the migrations: %v", err)
	}
	return types, opaque
}

func columnNames(list string) []string {
	var names []string
	for name := range strings.SplitSeq(list, ",") {
		names = append(names, strings.Trim(strings.TrimSpace(name), `"`))
	}
	return names
}

var sqlUnion = regexp.MustCompile(`(?i)\bUNION(?:\s+(?:ALL|DISTINCT))?\b`)

// auditStatementTypes reads the literals in the entity_type position of each
// SELECT or VALUES row an INSERT takes.
func auditStatementTypes(body string, column int) []string {
	if column < 0 {
		return nil
	}
	var rows []string
	for _, part := range sqlUnion.Split(body, -1) {
		trimmed := strings.TrimSpace(part)
		switch {
		case strings.HasPrefix(strings.ToUpper(trimmed), "SELECT"):
			list := trimmed[len("SELECT"):]
			if from := topLevelFrom(list); from >= 0 {
				list = list[:from]
			}
			rows = append(rows, list)
		case strings.HasPrefix(strings.ToUpper(trimmed), "VALUES"):
			for _, group := range gatekit.SplitSQLList(trimmed[len("VALUES"):]) {
				rows = append(rows, strings.TrimSuffix(strings.TrimPrefix(strings.TrimSpace(group), "("), ")"))
			}
		default:
			// An arm this cannot read, such as a CTE or a parenthesized query,
			// may name any type.
			return nil
		}
	}
	var out []string
	for _, row := range rows {
		items := gatekit.SplitSQLList(row)
		if column >= len(items) {
			return nil
		}
		values, ok := entityLiterals(items[column])
		if !ok {
			return nil
		}
		out = append(out, values...)
	}
	return out
}

var (
	sqlCaseKeyword = regexp.MustCompile(`(?i)\b(?:WHEN|THEN|ELSE|END)\b`)
	sqlLiteralOnly = regexp.MustCompile(`^'([a-z_]+)'(?:::\w+)?$`)
)

// entityLiterals reads an entity_type expression that can only be literals: one
// literal, or a CASE each of whose results is one.
func entityLiterals(item string) ([]string, bool) {
	item = strings.TrimSpace(item)
	if m := sqlLiteralOnly.FindStringSubmatch(item); m != nil {
		return []string{m[1]}, true
	}
	if !strings.HasPrefix(strings.ToUpper(item), "CASE") {
		return nil, false
	}
	keywords := sqlCaseKeyword.FindAllStringIndex(item, -1)
	var out []string
	for i, at := range keywords {
		word := strings.ToUpper(item[at[0]:at[1]])
		if (word != "THEN" && word != "ELSE") || i+1 == len(keywords) {
			continue
		}
		m := sqlLiteralOnly.FindStringSubmatch(strings.TrimSpace(item[at[1]:keywords[i+1][0]]))
		if m == nil {
			return nil, false
		}
		out = append(out, m[1])
	}
	return out, len(out) > 0
}

// topLevelFrom is where a select list's `FROM` starts, outside parentheses.
func topLevelFrom(list string) int {
	depth := 0
	upper := strings.ToUpper(list)
	quoted := false
	for i := 0; i < len(list); i++ {
		switch list[i] {
		case '\'':
			quoted = !quoted
		case '(':
			depth++
		case ')':
			depth--
		}
		if !quoted && depth == 0 && strings.HasPrefix(upper[i:], "FROM") && (i == 0 || !isWordByte(list[i-1])) &&
			(i+4 == len(list) || !isWordByte(list[i+4])) {
			return i
		}
	}
	return -1
}
