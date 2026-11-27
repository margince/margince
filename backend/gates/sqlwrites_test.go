// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package gates

import (
	"slices"
	"strings"
	"sync"
)

func sqlWriteTargets(literal string) []string {
	writes := sqlWrites(literal)
	tables := make([]string, 0, len(writes))
	for _, write := range writes {
		tables = append(tables, write.table)
	}
	return tables
}

// An empty answer is unknown. Callers must never treat it as proof that
// protected data stays unchanged.
//
// Several censuses judge the same statements, so each is read once per test
// binary. Every caller gets its own copy of the answer.
func sqlWrites(literal string) []sqlTarget {
	held, found := sqlWritesRead.Load(literal)
	writes, read := held.([]sqlTarget)
	if !found || !read {
		// Read from a clone, so neither the key nor a name cut from it holds
		// the whole file a statement may have been cut from.
		statement := strings.Clone(literal)
		writes = readSQLWrites(statement)
		sqlWritesRead.Store(statement, writes)
	}
	copied := slices.Clone(writes)
	for i := range copied {
		copied[i].cols = slices.Clone(copied[i].cols)
	}
	return copied
}

var sqlWritesRead sync.Map // statement -> []sqlTarget

func readSQLWrites(literal string) []sqlTarget {
	tokens := sqlToken.FindAllString(stripNoise(literal), -1)
	var writes []sqlTarget
	for i, word := range tokens {
		switch strings.ToLower(word) {
		case "insert", "delete", "merge":
			companion := "into"
			if strings.EqualFold(word, "delete") {
				companion = "from"
			}
			if i+1 >= len(tokens) || !strings.EqualFold(tokens[i+1], companion) {
				continue
			}
			table, next := writeSQLName(tokens, i+2)
			if table == "" {
				continue
			}
			switch strings.ToLower(word) {
			case "insert":
				writes = append(writes, sqlTarget{table, "insert", insertTargetColumns(tokens, next)})
			case "delete":
				writes = append(writes, sqlTarget{table, "delete", nil})
			case "merge":
				using := next
				if using < len(tokens) && strings.EqualFold(tokens[using], "as") {
					using += 2
				} else if using < len(tokens) && !strings.EqualFold(tokens[using], "using") {
					using++
				}
				if using >= len(tokens) || !strings.EqualFold(tokens[using], "using") {
					continue
				}
				writes = append(writes, mergeWriteTargets(tokens, next, table)...)
			}
		case "update":
			if i > 0 && (strings.EqualFold(tokens[i-1], "for") || strings.EqualFold(tokens[i-1], "do")) {
				continue
			}
			table, next := writeSQLName(tokens, i+1)
			if table == "" {
				continue
			}
			if next < len(tokens) && tokens[next] == "*" {
				next++
			}
			if next < len(tokens) && strings.EqualFold(tokens[next], "as") {
				next += 2
			} else if next+1 < len(tokens) && isName(tokens[next]) && !strings.EqualFold(tokens[next], "set") {
				next++
			}
			if next >= len(tokens) || !strings.EqualFold(tokens[next], "set") {
				continue
			}
			writes = append(writes, sqlTarget{table, "update", assignmentColumns(tokens, next+1)})
		}
	}
	return writes
}

func writeSQLName(tokens []string, at int) (string, int) {
	if at < len(tokens) && strings.EqualFold(tokens[at], "only") {
		at++
	}
	if at >= len(tokens) || !isName(tokens[at]) {
		return "", at
	}
	identifierParts := []string{tokens[at]}
	at++
	for at+1 < len(tokens) && tokens[at] == "." && isName(tokens[at+1]) {
		identifierParts = append(identifierParts, tokens[at+1])
		at += 2
	}
	name := strings.ToLower(strings.ReplaceAll(strings.Join(identifierParts, "."), `"`, ""))
	return strings.TrimPrefix(name, "public."), at
}

func insertTargetColumns(tokens []string, at int) []string {
	if at >= len(tokens) || tokens[at] != "(" {
		return nil
	}
	end := matchingParen(tokens, at)
	if end <= at || tokens[end] != ")" {
		return nil
	}
	return targetColumnNames(tokens[at+1 : end])
}

func targetColumnNames(tokens []string) []string {
	var columns []string
	for i := 0; i < len(tokens); i++ {
		if !isName(tokens[i]) {
			continue
		}
		name, next := writeSQLName(tokens, i)
		if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
			name = name[dot+1:]
		}
		columns = append(columns, name)
		i = next - 1
	}
	return columns
}

func assignmentColumns(tokens []string, start int) []string {
	var columns []string
	depth, cases, lhs, inValue := 0, 0, start, false
	for i := start; i < len(tokens); i++ {
		word := tokens[i]
		if depth == 0 && cases == 0 && (word == ";" || strings.EqualFold(word, "where") || strings.EqualFold(word, "returning") || strings.EqualFold(word, "when")) {
			break
		}
		if strings.EqualFold(word, "case") {
			cases++
		}
		if strings.EqualFold(word, "end") && cases > 0 {
			cases--
		}
		switch word {
		case "(", "[":
			depth++
		case ")", "]":
			if depth == 0 {
				return columns
			}
			depth--
		case "=":
			if depth == 0 && !inValue {
				targets := targetColumnNames(tokens[lhs:i])
				if len(targets) == 0 || slices.Contains(tokens[lhs:i], "?") {
					return nil
				}
				columns = append(columns, targets...)
				inValue = true
			}
		case ",":
			if depth == 0 && cases == 0 {
				lhs = i + 1
				inValue = false
			}
		}
	}
	return columns
}

func mergeWriteTargets(tokens []string, start int, table string) []sqlTarget {
	var writes []sqlTarget
	for i := start; i < len(tokens) && tokens[i] != ";"; i++ {
		if i+1 < len(tokens) && strings.EqualFold(tokens[i], "update") && strings.EqualFold(tokens[i+1], "set") {
			writes = append(writes, sqlTarget{table, "update", assignmentColumns(tokens, i+2)})
		}
		if strings.EqualFold(tokens[i], "insert") {
			writes = append(writes, sqlTarget{table, "insert", insertTargetColumns(tokens, i+1)})
		}
		if strings.EqualFold(tokens[i], "delete") {
			writes = append(writes, sqlTarget{table, "delete", nil})
		}
	}
	return writes
}

func sqlWritesVerb(statement, verb string) bool {
	for _, write := range sqlWrites(statement) {
		if write.verb == verb {
			return true
		}
	}
	return false
}
