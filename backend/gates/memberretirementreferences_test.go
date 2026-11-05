// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

import (
	"go/ast"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

type memberGuard struct{ live, column string }

func TestEveryMemberForeignKeyHasARetirementPolicy(t *testing.T) {
	t.Parallel()
	file, err := gatekit.ParseFile("internal/compose/memberreferences.go", 0)
	if err != nil {
		t.Fatal(err)
	}
	constants := memberPolicyConstants(t)
	policies := map[string]memberGuard{}
	ast.Inspect(file, func(node ast.Node) bool {
		entry, ok := node.(*ast.KeyValueExpr)
		if !ok {
			return true
		}
		key, ok := entry.Key.(*ast.BasicLit)
		if !ok {
			return true
		}
		name := memberPolicyString(t, key, constants)
		if !strings.Contains(name, ".") {
			return true
		}
		policies[name] = readMemberGuard(t, entry.Value, constants)
		return false
	})
	catalog, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatal(err)
	}
	refs := regexp.MustCompile(`(?m)^public\.([^.\s]+)\.[^ \r\n]+ FOREIGN KEY \(([^)]+)\) REFERENCES app_user\(id\)`).FindAllStringSubmatch(string(catalog), -1)
	if len(refs) < 100 {
		t.Fatalf("member-reference census found only %d foreign keys", len(refs))
	}
	seen := map[string]bool{}
	for _, ref := range refs {
		key := ref[1] + "." + ref[2]
		if _, ok := policies[key]; !ok {
			t.Errorf("%s has no retirement policy", key)
		}
		seen[key] = true
	}
	for key := range policies {
		if !seen[key] {
			t.Errorf("%s is not an installed member reference", key)
		}
	}
	assertMemberWorkGuards(t, string(catalog), policies)
	assertNoticeWorkStates(t, string(catalog), policies["privacy_notice_case.owner_user_id"].live)
}

func memberPolicyConstants(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob("internal/compose/*.go")
	if err != nil {
		t.Fatal(err)
	}
	constants := map[string]string{}
	for _, path := range files {
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			spec, ok := node.(*ast.ValueSpec)
			if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
				return true
			}
			literal, ok := spec.Values[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			value, err := strconv.Unquote(literal.Value)
			if err == nil {
				constants[spec.Names[0].Name] = value
			}
			return true
		})
	}
	return constants
}

func memberPolicyString(t *testing.T, expr ast.Expr, constants map[string]string) string {
	t.Helper()
	switch value := expr.(type) {
	case *ast.BasicLit:
		text, err := strconv.Unquote(value.Value)
		if err != nil {
			t.Fatal(err)
		}
		return text
	case *ast.Ident:
		text, ok := constants[value.Name]
		if !ok {
			t.Fatalf("unknown policy constant %s", value.Name)
		}
		return text
	default:
		t.Fatalf("unsupported member policy expression %T", expr)
		return ""
	}
}

func readMemberGuard(t *testing.T, expr ast.Expr, constants map[string]string) memberGuard {
	t.Helper()
	composite, ok := expr.(*ast.CompositeLit)
	if !ok {
		t.Fatalf("member policy is %T", expr)
	}
	var guard memberGuard
	retained := false
	for _, field := range composite.Elts {
		pair, ok := field.(*ast.KeyValueExpr)
		if !ok {
			t.Fatal("member policy field has no name")
		}
		name, ok := pair.Key.(*ast.Ident)
		if !ok {
			t.Fatal("member policy field is not an identifier")
		}
		switch name.Name {
		case "disposition":
			retained = memberPolicyString(t, pair.Value, constants) == constants["referenceRetain"]
		case "live":
			guard.live = memberPolicyString(t, pair.Value, constants)
		case "guardColumn":
			guard.column = memberPolicyString(t, pair.Value, constants)
		}
	}
	if !retained && guard.live == "" {
		t.Fatal("an active member-reference policy needs a lifecycle predicate")
	}
	return guard
}

func assertMemberWorkGuards(t *testing.T, catalog string, policies map[string]memberGuard) {
	t.Helper()
	found := map[string]string{}
	pattern := regexp.MustCompile(`(?m)^public\.([a-z_]+)\.[^\n]+ CREATE TRIGGER [^\n]+ FOR EACH ROW WHEN (.+) EXECUTE FUNCTION refuse_retired_member_work\('([a-z_]+)'\)$`)
	for _, match := range pattern.FindAllStringSubmatch(catalog, -1) {
		found[match[1]+"."+match[3]] = normalizeMemberPredicate(match[2])
	}
	required := 0
	for reference, guard := range policies {
		if guard.live == "" {
			continue
		}
		required++
		table, column, _ := strings.Cut(reference, ".")
		if guard.column != "" {
			column = guard.column
		}
		expected := normalizeMemberPredicate(column + " IS NOT NULL AND (" + guard.live + ")")
		if found[table+"."+column] != expected {
			t.Errorf("%s needs installed lifecycle guard %q, got %q", reference, expected, found[table+"."+column])
		}
	}
	if required == 0 {
		t.Fatal("no active dependencies were classified")
	}
	if len(found) != required {
		t.Errorf("found %d installed guards for %d active dependencies", len(found), required)
	}
}

// PostgreSQL deparses `IN` as `ANY` arrays and adds casts and parentheses.
func normalizeMemberPredicate(predicate string) string {
	predicate = strings.ToLower(predicate)
	for _, pair := range [][2]string{{"::text", ""}, {"new.", ""}, {"<> all", "not in"}, {"= any", "in"}, {"array[", ""}} {
		predicate = strings.ReplaceAll(predicate, pair[0], pair[1])
	}
	return strings.Map(func(r rune) rune {
		if strings.ContainsRune("()[] \t\r\n", r) {
			return -1
		}
		return r
	}, predicate)
}

func assertNoticeWorkStates(t *testing.T, catalog, predicate string) {
	t.Helper()
	states := map[string]map[string]bool{}
	checks := regexp.MustCompile(`(?m)^public\.privacy_notice_case\.privacy_notice_case_(state|completion_shape) CHECK (.+)$`)
	quoted := regexp.MustCompile(`'([a-z_]+)'`)
	for _, check := range checks.FindAllStringSubmatch(catalog, -1) {
		values := map[string]bool{}
		for _, value := range quoted.FindAllStringSubmatch(check[2], -1) {
			values[value[1]] = true
		}
		states[check[1]] = values
	}
	if len(states) != 2 || len(states["state"]) == 0 || len(states["completion_shape"]) == 0 {
		t.Fatal("notice lifecycle checks were not fully read")
	}
	live := map[string]bool{}
	for _, value := range quoted.FindAllStringSubmatch(predicate, -1) {
		live[value[1]] = true
	}
	for state := range states["state"] {
		if live[state] == states["completion_shape"][state] {
			t.Errorf("notice state %s has the wrong retirement disposition", state)
		}
		delete(live, state)
	}
	if len(live) != 0 {
		t.Errorf("retirement predicate names unknown notice states: %v", live)
	}
}
