// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Whether an activity has happened by a moment, or falls in a period, is asked
// of occurredday.
//
// A day entry is stored at noon UTC of its date. So `occurred_at <= now()`
// calls a day entry dated today ahead until that noon. It calls a day meeting
// held once the noon passes. And `occurred_at >= $1 AND occurred_at < $2` over
// local midnights puts a Sunday east of UTC+12 in the next week. BegunSQL,
// MeetingTookPlaceSQL, HappenedSQL, AheadSQL and WithinSQL read the date in the
// installation zone. So an activity row's occurred_at ordered against anything
// else is a finding. That is a bind, the clock, a function of them, another
// column, or a bound Go joins on. A row compared with a row,
// `(occurred_at, id) > (x, y)`, is a keyset order, which neither question needs.
//
// Some comparisons are neither: a retention age, a walk's keyset bound, a
// clock-skew allowance, a row of another table. Each says so on its own line of
// SQL, as `-- occurred_at raw: <reason>`, so a second comparison added to a
// waived statement is a finding of its own.
//
// Go may also hand the column to a helper as text, `in("a.occurred_at")`, which
// then compares it where the census cannot see the alias. Only occurredday may
// take it. Any other call says why on its line or the one above, as
// `// occurred_at raw: <reason>`.
//
// The .sql files count too: every function and view body the migrations leave
// in the schema, a later definition replacing an earlier one. A body may carry
// the owner's SQL rendered verbatim, or employment.InPlaceForSQL built on it,
// which is the owner's and not a finding. A
// migration's other statements ran once, at the upgrade that shipped them; they
// judge no moment that is still to come.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// aliasedColumn is occurred_at under its alias if any, past a cast.
const aliasedColumn = `(?:\b([a-z_]\w*)\.)?\boccurred_at\b(?:\s*::\s*[a-z_]\w*)?`

// callArguments is a call's other arguments, one level of nesting deep.
const callArguments = `(?:[^()]|\([^()]*\))*`

// columnSpelling is occurred_at, under its alias if any: bare, in parentheses,
// or any argument of a call such as max() or coalesce().
const columnSpelling = `\b[a-z_]\w*\((?:` + callArguments + `,\s*)?` + aliasedColumn + `(?:\s*,` + callArguments + `)?\)|\(\s*` +
	aliasedColumn + `\s*\)|` + aliasedColumn

var (
	// occurredAtOwner is the one package allowed to compare occurred_at raw.
	occurredAtOwner = filepath.Join("internal", "shared", "kernel", "occurredday")
	// occurredAtColumn finds columnSpelling in a line of SQL.
	occurredAtColumn = regexp.MustCompile(`(?i)` + columnSpelling)
	// columnText is a literal that is nothing but the column, as a helper takes it.
	columnText = regexp.MustCompile(`(?i)^\s*(?:` + columnSpelling + `)\s*$`)
	// goWaiver is the waiver a Go comment carries for a column handed to a helper.
	goWaiver = regexp.MustCompile(`^(?://|/\*)\s*occurred_at raw:\s*(\S.*?)\s*(?:\*/)?$`)
	// orderAfter is an ordering operator after the column, past a cast.
	orderAfter = regexp.MustCompile(`(?i)^(?:\s*::\s*[a-z_]\w*)?\s*(<=|>=|<>|!=|<|>)`)
	// orderBefore is an ordering operator before the column. An arrow, `=>` or
	// `<>` is not one.
	orderBefore = regexp.MustCompile(`(?:^|[^-=<>!])(<=|>=|<|>)\s*$`)
	// tableAlias binds an alias to the table a `FROM`, `JOIN` or `UPDATE` names.
	tableAlias = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|UPDATE)\s+([a-z_]\w*)(?:\s+(?:AS\s+)?([a-z_]\w*))?`)
	// rawOccurredAtWaiver is the marker and the reason it must carry.
	rawOccurredAtWaiver = regexp.MustCompile(`--\s*occurred_at raw:\s*(\S.*)$`)
)

// sqlKeywords cannot be an alias: `FROM activity WHERE` binds none.
var sqlKeywords = map[string]bool{
	"where": true, "join": true, "left": true, "inner": true, "on": true, "set": true, "using": true,
	"group": true, "order": true, "limit": true, "union": true, "cross": true, "natural": true, "full": true,
	"right": true, "lateral": true, "as": true, "and": true, "or": true, "returning": true, "offset": true,
}

// minRawOccurredAtCompares is the floor under the Go census: fewer comparisons
// on activity rows than this means the walk reads a smaller tree than it should.
const minRawOccurredAtCompares = 50

// rawCompare is one raw comparison of an activity row's occurred_at.
type rawCompare struct {
	where  string
	waived bool
	reason string
	// swallows marks a waiver on a literal's last line, where the SQL comment
	// would swallow whatever the caller appends.
	swallows bool
	// handed marks the column passed to a helper as text, waived in Go.
	handed bool
}

func TestWhetherAnActivityHappenedIsAskedOfItsOwner(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return namesOccurredAt(file) },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	seen := 0
	root := gatekit.ModuleRoot(t)
	for _, src := range scope.Files(t) {
		if strings.HasPrefix(filepath.Clean(src.Path), occurredAtOwner) {
			continue
		}
		text, err := os.ReadFile(filepath.Join(root, src.Path)) // #nosec G304 -- a swept file of the trusted module tree
		if err != nil {
			t.Fatal(err)
		}
		found := rawOccurredAtCompares(src.Path, src.File, text)
		seen += len(found)
		judgeRawCompares(t, found)
	}
	if seen < minRawOccurredAtCompares {
		t.Errorf("the census found %d raw occurred_at comparisons, want at least %d: it reads a smaller tree than it should",
			seen, minRawOccurredAtCompares)
	}
}

func judgeRawCompares(t *testing.T, found []rawCompare) {
	t.Helper()
	for _, compare := range found {
		switch {
		case !compare.waived && compare.handed:
			t.Errorf("%s is handed to a helper as text, which compares it where the census cannot see. Pass the "+
				"alias to an occurredday function, or say why on its line or the one above: // occurred_at raw: <reason>",
				compare.where)
		case !compare.waived:
			t.Errorf("%s compares an activity's occurred_at with a moment raw. A day entry is stored at its "+
				"date's noon UTC, so ask occurredday.BegunSQL, MeetingTookPlaceSQL, HappenedSQL or AheadSQL, "+
				"and a period WithinSQL, FromSQL or BeforeSQL. If it is an age or a keyset bound, say so on "+
				"its line: -- occurred_at raw: <reason>", compare.where)
		case compare.swallows:
			t.Errorf("%s waives on the last line of its string, so the -- comment swallows any SQL joined after it; "+
				"end the line inside the string", compare.where)
		case len(strings.Fields(compare.reason)) < 3:
			t.Errorf("%s waives a raw occurred_at comparison with %q; say in a few words why the date does not matter",
				compare.where, compare.reason)
		}
	}
}

// tables is the alias bindings some SQL makes, and whether it names a table.
type tables struct {
	aliases       map[string]string
	namesActivity bool
	namesAny      bool
}

func (into *tables) read(sql string) {
	for _, bind := range tableAlias.FindAllStringSubmatch(sql, -1) {
		table := strings.ToLower(bind[1])
		into.namesAny = true
		into.namesActivity = into.namesActivity || table == "activity"
		if alias := strings.ToLower(bind[2]); alias != "" && !sqlKeywords[alias] {
			into.aliases[alias] = table
		}
	}
}

func tablesIn(sql ...string) tables {
	bound := tables{aliases: map[string]string{}}
	for _, text := range sql {
		bound.read(text)
	}
	return bound
}

// comparesActivity says whether a comparison's alias names an activity row.
// The nearest SQL that names tables answers: the literal, its declaration, then
// its file. An alias bound to no table counts.
func comparesActivity(alias string, scopes ...tables) bool {
	for _, scope := range scopes {
		if !scope.namesAny {
			continue
		}
		if alias == "" {
			return scope.namesActivity
		}
		if table, bound := scope.aliases[alias]; bound {
			return table == "activity"
		}
	}
	return true
}

// orderedAliases reads every ordering comparison of occurred_at on one line
// and returns the alias each one orders, "" for a bare column.
func orderedAliases(line string) []string {
	var out []string
	for _, at := range occurredAtColumn.FindAllStringSubmatchIndex(line, -1) {
		alias := matchedAlias(line, at)
		if after := orderAfter.FindStringSubmatchIndex(line[at[1]:]); after != nil {
			op := line[at[1]+after[2] : at[1]+after[3]]
			if op != "<>" && op != "!=" {
				out = append(out, alias)
			}
			continue
		}
		if orderBefore.MatchString(line[:at[0]]) {
			out = append(out, alias)
		}
	}
	return out
}

// matchedAlias is the alias an occurredAtColumn match at names, "" for none.
func matchedAlias(text string, at []int) string {
	for _, group := range []int{2, 4, 6} {
		if at[group] >= 0 {
			return strings.ToLower(text[at[group]:at[group+1]])
		}
	}
	return ""
}

// linesComparing judges each line of text and returns the raw comparisons on
// activity rows, scopes naming the tables nearest first.
func linesComparing(where, text string, scopes ...tables) []rawCompare {
	var out []rawCompare
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		for _, alias := range orderedAliases(line) {
			if !comparesActivity(alias, scopes...) {
				continue
			}
			compare := rawCompare{where: where + ": " + strings.TrimSpace(line)}
			if waiver := rawOccurredAtWaiver.FindStringSubmatch(line); waiver != nil {
				compare.waived, compare.reason, compare.swallows = true, waiver[1], i == len(lines)-1
			}
			out = append(out, compare)
		}
	}
	return out
}

// namesOccurredAt says whether any string literal in file names the column.
func namesOccurredAt(file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		if lit, ok := n.(*ast.BasicLit); ok {
			text, isString := gatekit.LiteralText(lit)
			found = found || isString && strings.Contains(text, "occurred_at")
		}
		return !found
	})
	return found
}

// rawOccurredAtCompares finds each raw as-of comparison of an activity row's
// occurred_at in file's string literals, and each time the column is handed to
// a helper outside occurredday. source is file's text, for the waivers' lines.
func rawOccurredAtCompares(path string, file *ast.File, source []byte) []rawCompare {
	var out []rawCompare
	whole := tablesIn(gatekit.SQLStatementsOf(file)...)
	waivers := goWaivers(file, source)
	for _, decl := range file.Decls {
		declared := tablesIn(gatekit.SQLStatementsOf(decl)...)
		ast.Inspect(decl, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.BasicLit:
				if text, isString := gatekit.LiteralText(node); isString && node.Kind == token.STRING {
					out = append(out, linesComparing(path, text, tablesIn(text), declared, whole)...)
				}
			case *ast.CallExpr:
				out = append(out, columnsHandedOver(path, node, waivers, sourceLine(file, source), declared, whole)...)
			}
			return true
		})
	}
	return out
}

// columnsHandedOver is each argument of call that is the column as text, where
// the callee is not occurredday's. A waiver on the call's line or the one above
// carries its reason.
func columnsHandedOver(path string, call *ast.CallExpr, waivers map[int]string, line func(token.Pos) int, scopes ...tables) []rawCompare {
	if callee, ok := call.Fun.(*ast.SelectorExpr); ok {
		if pkg, ok := callee.X.(*ast.Ident); ok && pkg.Name == "occurredday" {
			return nil
		}
	}
	var out []rawCompare
	for _, arg := range call.Args {
		lit, ok := arg.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			continue
		}
		text, isString := gatekit.LiteralText(lit)
		if !isString || !columnText.MatchString(text) {
			continue
		}
		// A bare name is a column a write sets, not a time a helper reads.
		alias := matchedAlias(text, occurredAtColumn.FindStringSubmatchIndex(text))
		if alias == "" || !comparesActivity(alias, scopes...) {
			continue
		}
		compare := rawCompare{where: path + ": " + text, handed: true}
		at := line(lit.Pos())
		for _, waived := range []int{at, at - 1} {
			if reason, found := waivers[waived]; found {
				compare.waived, compare.reason = true, reason
			}
		}
		out = append(out, compare)
	}
	return out
}

// goWaivers maps each line holding a Go waiver comment to its reason.
func goWaivers(file *ast.File, source []byte) map[int]string {
	line := sourceLine(file, source)
	out := map[int]string{}
	for _, group := range file.Comments {
		for _, comment := range group.List {
			if waiver := goWaiver.FindStringSubmatch(comment.Text); waiver != nil {
				out[line(comment.Pos())] = waiver[1]
			}
		}
	}
	return out
}

// sourceLine maps a position in file to its line in source.
func sourceLine(file *ast.File, source []byte) func(token.Pos) int {
	return func(pos token.Pos) int {
		offset := min(max(int(pos-file.FileStart), 0), len(source))
		return 1 + strings.Count(string(source[:offset]), "\n")
	}
}

// minLiveBodiesComparingOccurredAt is the floor under the .sql census: the
// last-activity clock functions each order an activity's occurred_at.
const minLiveBodiesComparingOccurredAt = 4

var (
	// sqlDefinition is a function or view a migration creates or drops.
	sqlDefinition = regexp.MustCompile(`(?is)\b(CREATE(?:\s+OR\s+REPLACE)?|DROP)\s+(FUNCTION|(?:MATERIALIZED\s+)?VIEW)\s+(?:IF\s+EXISTS\s+)?([a-z_."]+)`)
	// dollarQuote opens and closes a function body.
	dollarQuote = regexp.MustCompile(`\$[a-z_]*\$`)
)

// sqlFile is one migration's path and text; sqlBody is a definition's.
type sqlFile struct{ path, text string }

type sqlBody struct{ where, text, args string }

func TestNoMigrationBodyAsksWhetherAnActivityHappenedRaw(t *testing.T) {
	t.Parallel()
	byDir := map[string][]sqlFile{}
	err := filepath.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".sql") || strings.HasSuffix(path, ".down.sql") {
			return err
		}
		raw, err := os.ReadFile(path) // #nosec G304 G122 -- a .sql file from walking the trusted module tree
		byDir[filepath.Dir(path)] = append(byDir[filepath.Dir(path)], sqlFile{path: path, text: string(raw)})
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	comparing := 0
	for _, files := range byDir {
		bodies, overloads := liveDefinitions(files)
		for _, overload := range overloads {
			t.Errorf("%s: the census keys a definition on its name, so an overload would hide its sibling; "+
				"drop the old one first or teach liveDefinitions the argument types", overload)
		}
		for _, body := range bodies {
			if len(linesComparing(body.where, body.text, tablesIn(body.text))) > 0 {
				comparing++
			}
			judgeRawCompares(t, bodyCompares(body))
		}
	}
	if comparing < minLiveBodiesComparingOccurredAt {
		t.Errorf("the .sql census found %d live bodies ordering an activity's occurred_at, want at least %d: "+
			"it reads fewer migrations than it should", comparing, minLiveBodiesComparingOccurredAt)
	}
}

// liveDefinitions returns the function and view bodies one directory's
// migrations leave in the schema, read in version order. It keys on the name,
// and returns as overloads the definitions that replace a live one with other
// arguments, which Postgres would keep beside it.
func liveDefinitions(files []sqlFile) (bodies []sqlBody, overloads []string) {
	slices.SortFunc(files, func(a, b sqlFile) int { return strings.Compare(a.path, b.path) })
	live := map[string]sqlBody{}
	for _, file := range files {
		for _, at := range sqlDefinition.FindAllStringSubmatchIndex(file.text, -1) {
			name := strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(file.text[at[6]:at[7]], `"`, "")), "public.")
			if strings.EqualFold(file.text[at[2]:at[3]], "DROP") {
				delete(live, name)
				continue
			}
			body := sqlBody{
				where: file.path + " " + name,
				text:  definitionBody(file.text[at[1]:], file.text[at[4]:at[5]]),
				args:  argumentList(file.text[at[1]:]),
			}
			if earlier, ok := live[name]; ok && earlier.args != body.args {
				overloads = append(overloads, body.where)
			}
			live[name] = body
		}
	}
	for _, body := range live {
		bodies = append(bodies, body)
	}
	return bodies, overloads
}

// argumentList is the parenthesised list right after a definition's name,
// lower-cased with its spacing folded, or "" for a view.
func argumentList(rest string) string {
	trimmed := strings.TrimLeft(rest, " \t\n")
	if !strings.HasPrefix(trimmed, "(") {
		return ""
	}
	depth := 0
	for i, r := range trimmed {
		switch r {
		case '(':
			depth++
		case ')':
			depth--
		}
		if depth == 0 {
			return strings.Join(strings.Fields(strings.ToLower(trimmed[1:i])), " ")
		}
	}
	return ""
}

// definitionBody is a function's dollar-quoted body, or a view's query up to
// its semicolon.
func definitionBody(rest, kind string) string {
	if strings.EqualFold(kind, "FUNCTION") {
		if open := dollarQuote.FindStringIndex(rest); open != nil {
			tag := rest[open[0]:open[1]]
			if end := strings.Index(rest[open[1]:], tag); end >= 0 {
				return rest[open[1] : open[1]+end]
			}
		}
		return ""
	}
	query, _, _ := strings.Cut(rest, ";")
	return query
}

// bodyCompares judges a body once the owner's SQL rendered in it is taken out.
func bodyCompares(body sqlBody) []rawCompare {
	bound := tablesIn(body.text)
	text := body.text
	for alias, table := range bound.aliases {
		if table != "activity" {
			continue
		}
		// The bound built on the owner goes first, before its parts are taken out.
		var owned []string
		for edge, kind := range bound.aliases {
			if kind == "relationship" {
				owned = append(owned, employment.InPlaceForSQL(edge, alias))
			}
		}
		owned = append(owned, occurredday.HappenedSQL(alias, "now()"), occurredday.MeetingTookPlaceSQL(alias, "now()"),
			occurredday.BegunSQL(alias, "now()"), occurredday.AheadSQL(alias, "now()"),
			occurredday.EndSQL(alias), occurredday.StartSQL(alias))
		for _, rendered := range owned {
			text = strings.ReplaceAll(text, rendered, " ")
		}
	}
	return linesComparing(body.where, text, bound)
}

// A live body is judged, a replaced or dropped one is not, and the owner's
// rendered SQL in a body is the owner's.
func TestTheMigrationCensusReadsOnlyLiveBodies(t *testing.T) {
	t.Parallel()
	raw := "CREATE FUNCTION clock(id uuid) RETURNS timestamptz LANGUAGE sql AS $$\n" +
		"  SELECT max(a.occurred_at) FROM activity a WHERE a.id = id AND a.occurred_at <= now()\n$$;\n"
	owned := "CREATE OR REPLACE FUNCTION clock(id uuid) RETURNS timestamptz LANGUAGE sql AS $$\n" +
		"  SELECT max(a.occurred_at) FROM activity a WHERE a.id = id AND " + occurredday.HappenedSQL("a", "now()") + "\n$$;\n"
	view := "CREATE VIEW past AS SELECT a.id FROM activity a WHERE a.occurred_at <= now();\n"
	for name, c := range map[string]struct {
		files []sqlFile
		want  int
	}{
		"a raw body":            {[]sqlFile{{"m/1.up.sql", raw}}, 1},
		"a raw view":            {[]sqlFile{{"m/1.up.sql", view}}, 1},
		"replaced by the owner": {[]sqlFile{{"m/2.up.sql", owned}, {"m/1.up.sql", raw}}, 0},
		"the owner replaced":    {[]sqlFile{{"m/1.up.sql", owned}, {"m/2.up.sql", raw}}, 1},
		"dropped":               {[]sqlFile{{"m/1.up.sql", raw}, {"m/2.up.sql", "DROP FUNCTION IF EXISTS clock(uuid);"}}, 0},
	} {
		got := 0
		bodies, _ := liveDefinitions(c.files)
		for _, body := range bodies {
			got += len(bodyCompares(body))
		}
		if got != c.want {
			t.Errorf("%s: the census found %d raw comparisons, want %d", name, got, c.want)
		}
	}
	overload := "CREATE FUNCTION clock(id uuid, at timestamptz) RETURNS timestamptz LANGUAGE sql AS $$ SELECT at $$;\n"
	if _, found := liveDefinitions([]sqlFile{{"m/1.up.sql", raw}, {"m/2.up.sql", overload}}); len(found) != 1 {
		t.Errorf("an overload replaced its sibling unseen: %v", found)
	}
	if _, found := liveDefinitions([]sqlFile{{"m/1.up.sql", raw}, {"m/2.up.sql", owned}}); len(found) != 0 {
		t.Errorf("a replacement with the same arguments read as an overload: %v", found)
	}
}

// The census sees each as-of spelling, reads past other tables, and holds a
// waiver to its own line.
func TestTheOccurredAtCensusSeesEachRawComparison(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{
		"now on the right":          "SELECT 1 FROM activity a WHERE a.occurred_at <= now()",
		"ahead of a bound":          "SELECT 1 FROM activity a WHERE a.occurred_at > $2",
		"sprintf placeholder":       "SELECT 1 FROM activity a WHERE a.occurred_at <= $%[3]d",
		"bound on the left":         "SELECT 1 FROM activity m WHERE $4::timestamptz >= m.occurred_at",
		"bare column":               "SELECT 1 FROM activity WHERE occurred_at < now()",
		"fragment of no table":      "AND x.occurred_at <= $%d",
		"bound joined from Go":      "SELECT 1 FROM activity a WHERE a.occurred_at <= ",
		"ahead joined from Go":      "SELECT 1 FROM activity a WHERE a.occurred_at > ",
		"pgx named argument":        "SELECT 1 FROM activity a WHERE a.occurred_at <= @as_of",
		"coalesce bound":            "SELECT 1 FROM activity a WHERE a.occurred_at <= coalesce($1, now())",
		"greatest bound":            "SELECT 1 FROM activity a WHERE a.occurred_at > greatest($1, $2)",
		"another column":            "SELECT 1 FROM activity a JOIN deal d ON a.occurred_at <= d.closed_at",
		"column on the left":        "SELECT 1 FROM activity a JOIN deal d ON d.closed_at >= a.occurred_at",
		"before a bound":            "SELECT 1 FROM activity a WHERE a.occurred_at < $1",
		"before a named arg":        "SELECT 1 FROM activity a WHERE a.occurred_at < @before",
		"before another column":     "SELECT 1 FROM activity a JOIN deal d ON a.occurred_at < d.closed_at",
		"latest of them":            "SELECT 1 FROM activity a GROUP BY a.id HAVING max(a.occurred_at) <= $1",
		"first in a coalesce":       "SELECT 1 FROM activity a WHERE coalesce(a.occurred_at, a.created_at) <= $1",
		"last in a coalesce":        "SELECT 1 FROM activity a WHERE COALESCE(h.scheduled_start, a.occurred_at) >= $1",
		"amid nested arguments":     "SELECT 1 FROM activity a WHERE greatest(lower(x), a.occurred_at, now()) < $1",
		"cast to a date":            "SELECT 1 FROM activity a WHERE a.occurred_at::date <= current_date",
		"from now on":               "SELECT 1 FROM activity a WHERE a.occurred_at >= now()",
		"a lower edge":              "SELECT 1 FROM activity a WHERE a.occurred_at >= $1",
		"a lower edge after a cast": "SELECT 1 FROM activity a WHERE a.occurred_at::date >= $1",
	} {
		if got := seededCompares(t, sql); len(got) != 1 || got[0].waived {
			t.Errorf("%s: the census found %+v in %q, want one unwaived comparison", name, got, sql)
		}
	}
	for name, sql := range map[string]string{
		"another table":      "SELECT 1 FROM audit_log au WHERE au.occurred_at <= $1",
		"another bare table": "DELETE FROM capture_trace WHERE occurred_at < now() - $1::interval",
		"not equal":          "SELECT 1 FROM activity a WHERE a.occurred_at <> now()",
		"a keyset row":       "SELECT 1 FROM activity a WHERE (a.occurred_at, a.id) > ($1, $2)",
		"a json arrow":       "SELECT a.after->>'x' FROM activity a WHERE a.after->>'at' = a.occurred_at::text",
	} {
		if got := seededCompares(t, sql); len(got) != 0 {
			t.Errorf("%s: the census flagged %+v in %q", name, got, sql)
		}
	}
	for name, sql := range map[string]string{
		"a period's edges":       "SELECT 1 FROM activity a WHERE a.occurred_at >= $1 AND a.occurred_at < $2",
		"a period up to now":     "SELECT 1 FROM activity a WHERE a.occurred_at >= $1 AND a.occurred_at < now()",
		"another alias's period": "SELECT 1 FROM activity a JOIN activity b ON b.id = a.id WHERE b.occurred_at >= $1 AND a.occurred_at < $2",
	} {
		if got := seededCompares(t, sql); len(got) != 2 {
			t.Errorf("%s: the census found %+v in %q, want both edges", name, got, sql)
		}
	}
	joined := "package p\nfunc f(b string) string { return `SELECT 1 FROM activity a WHERE a.occurred_at >= ` + b + ` AND a.occurred_at < ` + b }\n"
	if got := sourceCompares(t, joined); len(got) != 2 {
		t.Errorf("a period whose edges Go joins on was not flagged twice: %+v", got)
	}
	fragment := "package p\nconst q = `SELECT 1 FROM audit_log a WHERE `\nfunc f() string { return \" AND a.occurred_at <= \" }\n"
	if got := sourceCompares(t, fragment); len(got) != 0 {
		t.Errorf("a fragment whose file binds its alias to audit_log was flagged: %+v", got)
	}
	waived := "SELECT 1 FROM activity a\n WHERE a.occurred_at > $1 -- occurred_at raw: the window's lower edge\n AND a.occurred_at <= now()"
	got := seededCompares(t, waived)
	if len(got) != 2 || !got[0].waived || got[1].waived {
		t.Errorf("a waiver reached past its own line or missed it: %+v", got)
	}
	last := "SELECT 1 FROM activity a WHERE a.occurred_at > $1 -- occurred_at raw: the window has a lower edge"
	if got := seededCompares(t, last); len(got) != 1 || !got[0].swallows {
		t.Errorf("a waiver ending its string passed: %+v", got)
	}
}

// A column handed to a helper as text is a finding wherever it sits among the
// arguments, unless occurredday takes it or a Go comment beside it waives it.
func TestTheCensusSeesAColumnHandedToAHelper(t *testing.T) {
	t.Parallel()
	handed := map[string]struct {
		source string
		want   int
	}{
		"a column a helper takes": {"package p\nconst q = `FROM activity a`\nfunc f() string { return in(\"a.occurred_at\") }\n", 1},
		"second of its arguments": {"package p\nconst q = `FROM activity a`\nfunc f() string { return x.Between(\"y\", \"a.occurred_at\") }\n", 1},
		"a call over the column":  {"package p\nconst q = `FROM activity a`\nfunc f() string { return in(\"coalesce(h.at, a.occurred_at)\") }\n", 1},
		"the owner takes it":      {"package p\nconst q = `FROM activity a`\nfunc f() string { return occurredday.StartAtSQL(\"a.occurred_at\", \"p\") }\n", 0},
		"a bare column name":      {"package p\nfunc f() { p.Set(\"occurred_at\", 1) }\n", 0},
		"another table's column":  {"package p\nconst q = `FROM audit_log a`\nfunc f() string { return in(\"a.occurred_at\") }\n", 0},
	}
	for name, c := range handed {
		if got := sourceCompares(t, c.source); len(got) != c.want || (c.want > 0 && got[0].waived) {
			t.Errorf("%s: the census found %+v, want %d unwaived", name, got, c.want)
		}
	}
	for name, source := range map[string]string{
		"on the line above": "package p\nconst q = `FROM activity a`\nfunc f() string {\n\t// occurred_at raw: a signal's cited time has no precision\n\treturn in(\"a.occurred_at\")\n}\n",
		"inside the call":   "package p\nconst q = `FROM activity a`\nfunc f() string { return in(\"a.occurred_at\" /* occurred_at raw: a cited time has none */) }\n",
	} {
		if got := sourceCompares(t, source); len(got) != 1 || !got[0].waived {
			t.Errorf("%s: a waived hand-over read as %+v", name, got)
		}
	}
	far := "package p\nconst q = `FROM activity a`\n// occurred_at raw: too far above to count\n\nfunc f() string { return in(\"a.occurred_at\") }\n"
	if got := sourceCompares(t, far); len(got) != 1 || got[0].waived {
		t.Errorf("a waiver two lines up reached the call: %+v", got)
	}
}

// seededCompares runs the census over SQL planted in a constant.
func seededCompares(t *testing.T, sql string) []rawCompare {
	t.Helper()
	return sourceCompares(t, "package p\nconst q = `"+sql+"`\n")
}

// sourceCompares runs the census over a planted Go file.
func sourceCompares(t *testing.T, source string) []rawCompare {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "seeded.go", source, parser.ParseComments)
	if err != nil {
		t.Fatal(err)
	}
	return rawOccurredAtCompares("seeded.go", file, []byte(source))
}
