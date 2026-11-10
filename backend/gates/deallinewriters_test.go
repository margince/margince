// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

// A lines-priced deal has one value writer; every other writer must refuse
// line-owned columns and preserve its product projection.

package gates

import (
	"go/ast"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	lineValueSeam = "applyDealLineValueLocked"
	lineOwner     = "internal/modules/deals"
)

var linePatchSeams = []string{"applyDealPatchGuarded", "applyDealPatchLocked", lineValueSeam}

func TestEveryLinesDealMoneyWriterUsesItsGuard(t *testing.T) {
	t.Parallel()
	files := parsePackageFiles(t, lineOwner)
	constants := stringConstants(files)
	columns, err := stringSliceVar(files, "lineOwnedColumns", constants)
	if err != nil {
		t.Fatal(err)
	}
	modeColumns := []string{constants["commercialModeColumn"], constants["commercialRevisionColumn"]}
	columns = append(columns, modeColumns...)
	table := constants["dealTable"]
	if table == "" || modeColumns[0] == "" || modeColumns[1] == "" {
		t.Fatal("the deal table or its mode columns have no production name")
	}
	graph := packageCallGraph(t, lineOwner)
	projection := graph["projectDealLineProducts"]
	if projection == nil {
		t.Fatal("the line product projection was not read")
	}
	products := map[string]bool{}
	for _, statement := range projection.statements {
		for _, target := range sqlWrites(statement) {
			products[target.table] = true
		}
	}
	if len(products) != 1 {
		t.Fatalf("line products reach %d tables, expected one owned projection", len(products))
	}
	flows := linePackageFlows(t)
	for _, problem := range lineSeamProblems(flows[lineOwner], constants) {
		t.Error(problem)
	}
	productWrites, creates := 0, 0
	for dir, flow := range flows {
		for _, file := range flow.corpus {
			for _, statement := range sqlStrings(file, flow.constants) {
				name := ""
				for candidate, fn := range flow.functions {
					if fn.Pos() <= statement.pos && statement.pos < fn.End() {
						name = candidate
					}
				}
				site := lineSQLSite{dir, name, modeColumns}
				for _, problem := range lineSQLProblems(statement.text, table, columns, site) {
					t.Errorf("%s: %s", gatekit.SourceFileSet().Position(statement.pos), problem)
				}
				for _, write := range sqlWrites(statement.text) {
					if write.table == table && write.verb == "insert" {
						creates++
					}
					if products[write.table] {
						productWrites++
						if dir != lineOwner || name == "" || !flow.productsGuarded(name, statement.pos, lineValueSeam, map[string]bool{}) {
							t.Errorf("%s: product SQL bypasses its commercial-mode guard", gatekit.SourceFileSet().Position(statement.pos))
						}
					}
				}
			}
		}
	}
	seen := map[string]bool{}
	for dir, flow := range flows {
		for _, site := range flow.productPatchSites(products) {
			t.Errorf("%s: a product patch bypasses the product writer", gatekit.SourceFileSet().Position(site.call.Pos()))
		}
		for _, site := range flow.dealPatchSites(table) {
			if dir == lineOwner && flow.canonicalDealPatch(site) {
				seen[site.name] = true
				continue
			}
			t.Errorf("%s: %s writes a deal through a direct or unresolved patch; use the guarded deal writer", gatekit.SourceFileSet().Position(site.call.Pos()), site.name)
		}
	}
	for _, name := range linePatchSeams {
		if !seen[name] {
			t.Errorf("the writer census missed the production seam %s", name)
		}
	}

	if productWrites < 2 || creates == 0 {
		t.Fatalf("short line writer census: %d product writes, %d constructors", productWrites, creates)
	}
}

func linePackageFlows(t *testing.T) map[string]*lineLockFlow {
	t.Helper()
	groups := map[string][]*ast.File{}
	for _, source := range parseProductGoFiles(t) {
		if isIntegrationTagged(source.path) {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(source.path))
		groups[dir] = append(groups[dir], source.file)
	}
	if len(groups) < 50 {
		t.Fatalf("short product package census: %d", len(groups))
	}
	flows := map[string]*lineLockFlow{}
	for dir, files := range groups {
		// The patch primitive accepts every table; the obligation binds its callers.
		if dir != storekitOwned {
			flows[dir] = lineLocksOf(files)
		}
	}
	return flows
}

func lineMoneyColumns(written, protected []string) bool {
	if len(written) == 0 {
		return true
	}
	for _, column := range protected {
		if slices.Contains(written, column) {
			return true
		}
	}
	return false
}

type lineSQLSite struct {
	dir, function string
	modeColumns   []string
}

func lineSQLProblems(statement, table string, columns []string, site lineSQLSite) []string {
	var problems []string
	writes := sqlWrites(statement)
	for _, write := range writes {
		if write.table == table && write.verb == "insert" {
			if site.dir != lineOwner || site.function != "Store.createDealInTx" {
				problems = append(problems, "a deal insert bypasses its estimate constructor")
			} else {
				for _, column := range site.modeColumns {
					if strings.Contains(strings.ToLower(statement), column) || hasSQLSequence(statement, "on", "conflict") {
						problems = append(problems, "the constructor can replace a lines-priced deal")
					}
				}
			}
		}
		if write.table == table && write.verb == "update" && (strings.Contains(statement, "?") || lineMoneyColumns(write.cols, columns)) {
			problems = append(problems, "deal money or mode bypasses its guarded patch seam")
		}
	}
	if owningDir(site.dir) == lineOwner {
		tokens := sqlToken.FindAllString(statement, -1)
		for i, word := range tokens {
			mutation := strings.EqualFold(word, "update")
			if i > 0 {
				mutation = mutation || (strings.EqualFold(word, "into") && (strings.EqualFold(tokens[i-1], "insert") || strings.EqualFold(tokens[i-1], "merge"))) || (strings.EqualFold(word, "from") && strings.EqualFold(tokens[i-1], "delete"))
			}
			if mutation {
				if i+1 < len(tokens) && (tokens[i+1] == "?" || tokens[i+1] == "%") {
					problems = append(problems, "a computed deal-module mutation destination bypasses its guarded writer")
				}
			}
		}
	}
	return problems
}

func hasSQLSequence(statement string, words ...string) bool {
	tokens := sqlToken.FindAllString(stripNoise(statement), -1)
	for i := 0; i+len(words) <= len(tokens); i++ {
		match := true
		for j, word := range words {
			match = match && strings.EqualFold(tokens[i+j], word)
		}
		if match {
			return true
		}
	}
	return false
}

func lineSeamProblems(flow *lineLockFlow, constants map[string]string) []string {
	allowed := map[string]bool{"Store.settleDealLines": true, "Store.leaveLinesMode": true}
	seen := map[string]bool{}
	var problems []string
	if flow.external[lineValueSeam] {
		problems = append(problems, "the line-value writer escapes its closed settlement callers")
	}
	for name, fn := range flow.functions {
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			if flow.called(name, call) == lineValueSeam {
				seen[name] = true
				if !allowed[name] {
					problems = append(problems, name+" bypasses the line settlement writer")
				}
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if ok && strings.HasPrefix(method.Sel.Name, "Set") && len(call.Args) > 0 && !allowed[name] {
				columns := flow.origins(name, call.Args[0], 0)
				if columns[constants["commercialModeColumn"]] || columns[constants["commercialRevisionColumn"]] {
					problems = append(problems, name+" changes line mode outside settlement")
				}
			}
			return true
		})
	}
	for name := range allowed {
		if !seen[name] {
			problems = append(problems, "the line-value caller census missed "+name)
		}
	}
	return problems
}
