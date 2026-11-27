// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// A correction supersedes a day's rate and appends a new row, so the table holds
// several rows for one day. A read of fx_rate that picks no version can price a
// deal at either of them.
//
// Go reads pick through deals.RateVersionSQL. A statement that wants every version
// says so with the comment `every version` in its SQL. SQL functions and views
// are judged by their latest definition in the migration tree.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const allVersionsMarker = "every version"

// fxStatement is one SQL text in Go source, and whether a call to the version
// rule was assembled into it.
type fxStatement struct {
	text  string
	ruled bool
}

// fxVersionStatementsIn returns the SQL literals of a file. A literal is ruled
// when the concatenation or call it sits in also builds the version rule.
func fxVersionStatementsIn(file *ast.File) []fxStatement {
	var out []fxStatement
	joined := map[ast.Node]bool{}
	ast.Inspect(file, func(node ast.Node) bool {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op != token.ADD {
				return true
			}
			if text, ok := flattenConcatenation(typed, joined); ok {
				out = append(out, fxStatement{text: text, ruled: buildsVersionRule(typed)})
			}
			return false
		case *ast.CallExpr:
			if !buildsVersionRule(typed) {
				return true
			}
			for _, arg := range typed.Args {
				if lit, isLit := arg.(*ast.BasicLit); isLit {
					if text, isText := gatekit.LiteralText(lit); isText {
						joined[lit] = true
						out = append(out, fxStatement{text: text, ruled: true})
					}
				}
			}
		case *ast.BasicLit:
			if joined[typed] {
				return true
			}
			if text, isText := gatekit.LiteralText(typed); isText {
				out = append(out, fxStatement{text: text})
			}
		}
		return true
	})
	return out
}

// flattenConcatenation joins the string operands of one `+` chain and marks
// each literal it consumed, so the walk does not read it again on its own. A
// non-literal operand becomes a space, so two words it separates stay apart.
func flattenConcatenation(expr *ast.BinaryExpr, joined map[ast.Node]bool) (string, bool) {
	var parts []string
	var walk func(ast.Expr)
	walk = func(node ast.Expr) {
		switch typed := node.(type) {
		case *ast.BinaryExpr:
			if typed.Op == token.ADD {
				walk(typed.X)
				walk(typed.Y)
				return
			}
			parts = append(parts, " ")
		case *ast.BasicLit:
			if text, isText := gatekit.LiteralText(typed); isText {
				joined[typed] = true
				parts = append(parts, text)
				return
			}
			parts = append(parts, " ")
		default:
			parts = append(parts, " ")
		}
	}
	walk(expr)
	return strings.Join(parts, ""), len(parts) > 0
}

// buildsVersionRule reports whether the expression contains a call to the
// version rule.
func buildsVersionRule(node ast.Node) bool {
	found := false
	ast.Inspect(node, func(inner ast.Node) bool {
		call, isCall := inner.(*ast.CallExpr)
		if !isCall {
			return !found
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			found = found || fn.Name == "RateVersionSQL"
		case *ast.SelectorExpr:
			found = found || fn.Sel.Name == "RateVersionSQL"
		}
		return !found
	})
	return found
}

// unversionedGoRead reports a statement that reads fx_rate without choosing a
// version.
func unversionedGoRead(statement fxStatement) bool {
	return readsFXRate.MatchString(statement.text) && !statement.ruled &&
		!strings.Contains(statement.text, "superseded_at") && !strings.Contains(statement.text, allVersionsMarker)
}

func TestEveryGoReadOfTheRateSheetPicksAVersion(t *testing.T) {
	t.Parallel()
	var offences []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		for _, statement := range fxVersionStatementsIn(file) {
			if !readsFXRate.MatchString(statement.text) {
				continue
			}
			judged++
			if unversionedGoRead(statement) {
				offences = append(offences, filepath.ToSlash(path))
			}
		}
	}
	if judged < 8 {
		t.Fatalf("only %d reads of fx_rate were found, so this census has gone short", judged)
	}
	if len(offences) > 0 {
		t.Errorf("these files read fx_rate without picking a version, so a corrected day can be priced at either row:\n  %s\n"+
			"Build the clause with deals.RateVersionSQL, or write `/* %s */` in the SQL when every version is the point.",
			strings.Join(offences, "\n  "), allVersionsMarker)
	}
}

func TestTheGoVersionDetectorSeesEachShape(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		what, src string
		fires     bool
	}{
		{"a bare read", "package p\nvar q = `SELECT rate FROM fx_rate WHERE rate_date <= $1`\n", true},
		{"a read that names the column", "package p\nvar q = `SELECT rate FROM fx_rate WHERE superseded_at IS NULL`\n", false},
		{"a read with the rule built in", "package p\nvar q = `SELECT rate FROM fx_rate WHERE ` + RateVersionSQL(\"\", \"\")\n", false},
		{"a Sprintf given the rule", "package p\nvar q = fmt.Sprintf(`SELECT 1 FROM fx_rate fr WHERE %s`, deals.RateVersionSQL(\"fr\", \"\"))\n", false},
		{"a read that says it wants every version", "package p\nvar q = `SELECT max(created_at) FROM fx_rate /* every version */`\n", false},
		{"a concatenated read with a variable and no rule", "package p\nvar q = `SELECT rate FROM fx_rate WHERE x = ` + other\n", true},
	} {
		file, err := parser.ParseFile(gatekit.SourceFileSet(), "probe.go", probe.src, 0)
		if err != nil {
			t.Fatalf("parsing %q: %v", probe.what, err)
		}
		fired := false
		for _, statement := range fxVersionStatementsIn(file) {
			fired = fired || unversionedGoRead(statement)
		}
		if fired != probe.fires {
			t.Errorf("%s: flagged = %t, want %t", probe.what, fired, probe.fires)
		}
	}
}

var (
	sqlFunctionStart = regexp.MustCompile(`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?FUNCTION\s+([\w.]+)\s*\(`)
	sqlViewStart     = regexp.MustCompile(`(?is)CREATE\s+(?:OR\s+REPLACE\s+)?VIEW\s+([\w.]+)\s+AS\b`)
	sqlDropObject    = regexp.MustCompile(`(?is)DROP\s+(FUNCTION|VIEW)\s+(?:IF\s+EXISTS\s+)?([\w.]+)`)
	sqlDollarOpen    = regexp.MustCompile(`\$\w*\$`)
	rateSheetRead    = regexp.MustCompile(`(?i)\b(?:from|join)\s+fx_rate\b`)
	currentRateRule  = regexp.MustCompile(`(?i)superseded_at\s+IS\s+NULL`)
)

// latestSQLDefinitions replays the migrations in order and returns the final
// body of each function and view, keyed by kind and name. A later definition
// replaces an earlier one, and a drop removes it.
func latestSQLDefinitions(t *testing.T) map[string]string {
	t.Helper()
	files, err := gatekit.Glob("migrations/core/*.up.sql")
	if err != nil || len(files) == 0 {
		t.Fatalf("listing the core migrations: %v (%d files)", err, len(files))
	}
	sort.Strings(files)
	latest := map[string]string{}
	for _, file := range files {
		raw, err := gatekit.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		applyDefinitions(string(raw), latest)
	}
	return latest
}

// applyDefinitions folds one migration's functions, views and drops into latest.
func applyDefinitions(sql string, latest map[string]string) {
	type event struct {
		at   int
		kind string
		name string
		body string
	}
	var events []event
	for _, m := range sqlFunctionStart.FindAllStringSubmatchIndex(sql, -1) {
		rest := sql[m[1]:]
		open := sqlDollarOpen.FindStringIndex(rest)
		if open == nil {
			continue
		}
		delim := rest[open[0]:open[1]]
		closeAt := strings.Index(rest[open[1]:], delim)
		if closeAt < 0 {
			continue
		}
		events = append(events, event{at: m[0], kind: "function", name: sql[m[2]:m[3]], body: rest[open[1] : open[1]+closeAt]})
	}
	for _, m := range sqlViewStart.FindAllStringSubmatchIndex(sql, -1) {
		end := strings.Index(sql[m[1]:], ";")
		if end < 0 {
			end = len(sql) - m[1]
		}
		events = append(events, event{at: m[0], kind: "view", name: sql[m[2]:m[3]], body: sql[m[1] : m[1]+end]})
	}
	for _, m := range sqlDropObject.FindAllStringSubmatchIndex(sql, -1) {
		events = append(events, event{at: m[0], kind: strings.ToLower(sql[m[2]:m[3]]), name: sql[m[4]:m[5]]})
	}
	sort.Slice(events, func(i, j int) bool { return events[i].at < events[j].at })
	for _, e := range events {
		key := e.kind + " " + strings.ToLower(e.name)
		if e.body == "" {
			delete(latest, key)
			continue
		}
		latest[key] = e.body
	}
}

// unversionedSQLRead reports a body that reads fx_rate more often than it
// picks the current row.
func unversionedSQLRead(body string) bool {
	return len(rateSheetRead.FindAllString(body, -1)) > len(currentRateRule.FindAllString(body, -1))
}

func TestEverySQLReaderOfTheRateSheetPicksTheCurrentVersion(t *testing.T) {
	t.Parallel()
	var offences []string
	readers := 0
	for name, body := range latestSQLDefinitions(t) {
		if !rateSheetRead.MatchString(body) {
			continue
		}
		readers++
		if unversionedSQLRead(body) {
			offences = append(offences, name)
		}
	}
	sort.Strings(offences)
	if readers < 2 {
		t.Fatalf("only %d SQL readers of fx_rate were found, so this census has gone short", readers)
	}
	if len(offences) > 0 {
		t.Errorf("these SQL objects read fx_rate in their latest definition without `superseded_at IS NULL`:\n  %s\n"+
			"Redefine each in a new migration. Never edit a shipped one.", strings.Join(offences, "\n  "))
	}
}

func TestTheSQLVersionDetectorSeesEachShape(t *testing.T) {
	t.Parallel()
	latest := map[string]string{}
	applyDefinitions("CREATE FUNCTION f(a date) RETURNS int AS $fn$ SELECT 1 FROM fx_rate r $fn$;", latest)
	if !unversionedSQLRead(latest["function f"]) {
		t.Error("a function reading fx_rate with no version rule was not flagged")
	}
	applyDefinitions("CREATE OR REPLACE FUNCTION f(a date) RETURNS int AS $fn$ SELECT 1 FROM fx_rate r WHERE r.superseded_at IS NULL $fn$;", latest)
	if unversionedSQLRead(latest["function f"]) {
		t.Error("a later definition with the rule still counted as the earlier one")
	}
	applyDefinitions("DROP FUNCTION f(date);", latest)
	if _, kept := latest["function f"]; kept {
		t.Error("a dropped function stayed in the latest definitions")
	}
}
