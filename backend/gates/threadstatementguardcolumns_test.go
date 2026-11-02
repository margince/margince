// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build !integration

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"slices"
	"strings"
	"testing"
)

func threadGuardColumnMismatch(statement string, schema *emSchema) string {
	if schema == nil {
		return ""
	}
	tokens := threadStatementTokens(statement)
	regions := []threadQueryRegion{{start: 0, end: len(tokens)}}
	derived := map[string]bool{}
	for at, token := range tokens {
		if token != "(" || at+1 >= len(tokens) || tokens[at+1] != "select" && tokens[at+1] != "with" {
			continue
		}
		region := threadQueryRegion{start: at + 1, end: matchingParen(tokens, at)}
		regions = append(regions, region)
		if name := threadDerivedName(tokens, region); name != "" {
			derived[name] = true
		}
	}
	for at, token := range tokens {
		if token != "activity_thread_available" {
			continue
		}
		if at+1 >= len(tokens) || tokens[at+1] != "(" {
			return "unresolved availability call"
		}
		closing, depth := -1, 0
		for position := at + 1; position < len(tokens); position++ {
			if tokens[position] == "(" {
				depth++
			}
			if tokens[position] == ")" {
				depth--
				if depth == 0 {
					closing = position
					break
				}
			}
		}
		if closing < 0 {
			return "unresolved availability argument"
		}
		if closing != at+3 {
			continue
		}
		parts := strings.Split(tokens[at+2], ".")
		if len(parts) != 2 || parts[1] != "thread_key" {
			continue
		}
		region := regions[0]
		for _, candidate := range regions {
			if at >= candidate.start && at < candidate.end && candidate.end-candidate.start < region.end-region.start {
				region = candidate
			}
		}
		for position := region.start; position < region.end; position++ {
			if tokens[position] == "(" {
				position = matchingParen(tokens, position)
				continue
			}
			if !slices.Contains([]string{"from", "join", "using", "update", "into"}, tokens[position]) {
				continue
			}
			for _, tableAt := range tableTargets(tokens, position) {
				if tableAt >= len(tokens) {
					return "unresolved availability relation"
				}
				tableParts := strings.Split(tokens[tableAt], ".")
				name := tableParts[len(tableParts)-1]
				if len(tableParts) == 1 && derived[name] || threadTableAlias(tokens, tableAt) != parts[0] {
					continue
				}
				table, known := schema.tables[name]
				if !known {
					continue
				}
				present := false
				for _, column := range table.columns {
					if column.name == "thread_key" {
						present = true
					}
				}
				if !present {
					return parts[0] + ".thread_key (" + name + ")"
				}
			}
		}
	}
	return ""
}

func TestAvailabilityGuardsUseColumnsOfTheirActualPhysicalAlias(t *testing.T) {
	t.Parallel()
	schema := parseHeadCatalog(t)
	for _, check := range []struct {
		sql     string
		invalid bool
	}{
		{`SELECT id FROM attachment a WHERE activity_thread_available(a.thread_key)`, true},
		{`SELECT activity_thread_available(a.thread_key) FROM`, true},
		{`SELECT activity_thread_available(a.thread_key`, true},
		{`SELECT id FROM activity a WHERE activity_thread_available(a.thread_key)`, false},
		{`SELECT activity_thread_available(@key)`, false},
		{`SELECT EXISTS(SELECT 1 FROM activity a WHERE NOT activity_thread_available(a.thread_key))`, false},
		{`SELECT id FROM comms_outbound o WHERE activity_thread_available(o.thread_key)`, false},
		{`WITH attachment AS (SELECT thread_key FROM activity) SELECT * FROM attachment a WHERE activity_thread_available(a.thread_key)`, false},
		{`SELECT * FROM (SELECT thread_key FROM activity) a WHERE activity_thread_available(a.thread_key)`, false},
	} {
		if got := threadGuardColumnMismatch(check.sql, schema); (got != "") != check.invalid {
			t.Errorf("column mismatch=%q,want invalid=%t: %s", got, check.invalid, check.sql)
		}
	}
}

func TestQualifiedAvailabilityTyposCannotEscapeTheStatementSubjects(t *testing.T) {
	t.Parallel()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", `package fixture; func read(tx DB,ctx Context){tx.Query(ctx,"SELECT id FROM attachment a WHERE activity_thread_available(a.thread_key)")}`, 0)
	if err != nil {
		t.Fatal(err)
	}
	if !threadFileHasActivitySQL("fixture.go", file) {
		t.Fatal("availability query was absent from the subject corpus")
	}
	pkg := threadSQLPackageOf([]*ast.File{file})
	pkg.schema = parseHeadCatalog(t)
	if findings := pkg.unguardedQueries(pkg.functions["read"]); len(findings) != 1 {
		t.Fatalf("guard column failures=%v", findings)
	}
}
