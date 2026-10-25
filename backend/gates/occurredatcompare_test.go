// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Whether an activity has happened by a moment is asked of occurredday.
//
// A day entry is stored at noon UTC of its date. So `occurred_at <= now()`
// calls a day entry dated today ahead until that noon. It calls a day meeting
// held once the noon passes. occurredday.BegunSQL, MeetingTookPlaceSQL, HappenedSQL
// and AheadSQL read the date in the installation zone. So an activity row's
// occurred_at ordered against anything else is a finding: a bind, the clock,
// a function of them, another column, or a bound Go joins on.
//
// `occurred_at >= x` is a window's lower edge unless x is the clock. A bare
// `occurred_at < x` is "before x", and passes only beside a `>= y` on the same
// column in one statement, the two edges of a period. A row compared with a
// row, `(occurred_at, id) > (x, y)`, is a keyset order, which "has it happened" never needs.
//
// Some comparisons are not "has it happened": a window's edge, a retention age, a
// walk's keyset bound, a clock-skew allowance. Each says so on its own line of
// SQL, as `-- occurred_at raw: <reason>`, so a second comparison added to a
// waived statement is a finding of its own.
//
// The .sql files count too: every function and view body the migrations leave
// in the schema, a later definition replacing an earlier one. A body may carry
// the owner's SQL rendered verbatim, which is the owner's and not a finding. A
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
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// aliasedColumn is occurred_at under its alias if any, past a cast.
const aliasedColumn = `(?:\b([a-z_]\w*)\.)?\boccurred_at\b(?:\s*::\s*[a-z_]\w*)?`

const clockWords = `(?:now\(\)|current_timestamp|current_date|localtimestamp|statement_timestamp\(\)|transaction_timestamp\(\)|clock_timestamp\(\))`

var (
	// occurredAtOwner is the one package allowed to compare occurred_at raw.
	occurredAtOwner = filepath.Join("internal", "shared", "kernel", "occurredday")
	// occurredAtColumn is occurred_at, under its alias if any: bare, in
	// parentheses, or first in a call such as max() or coalesce().
	occurredAtColumn = regexp.MustCompile(`(?i)\b[a-z_]\w*\(\s*` + aliasedColumn + `(?:\s*,[^()]*)?\)|\(\s*` + aliasedColumn + `\s*\)|` + aliasedColumn)
	// orderAfter is an ordering operator after the column, past a cast.
	orderAfter = regexp.MustCompile(`(?i)^(?:\s*::\s*[a-z_]\w*)?\s*(<=|>=|<>|!=|<|>)`)
	// orderBefore is an ordering operator before the column. An arrow, `=>` or
	// `<>` is not one.
	orderBefore = regexp.MustCompile(`(?:^|[^-=<>!])(<=|>=|<|>)\s*$`)
	clockFirst  = regexp.MustCompile(`(?i)^[\s(]*` + clockWords)
	clockLast   = regexp.MustCompile(`(?i)` + clockWords + `[\s)]*$`)
	// tableAlias binds an alias to the table a `FROM`, `JOIN` or `UPDATE` names.
	tableAlias = regexp.MustCompile(`(?i)\b(?:FROM|JOIN|UPDATE)\s+([a-z_]\w*)(?:\s+(?:AS\s+)?([a-z_]\w*))?`)
	// rawOccurredAtWaiver is the marker and the reason it must carry.
	rawOccurredAtWaiver = regexp.MustCompile(`--\s*occurred_at raw:\s*(\S.*)$`)
)

// flipped is the operator read with the column on its left.
func flipped(op string) string {
	return strings.NewReplacer("<", ">", ">", "<").Replace(op)
}

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
}

func TestWhetherAnActivityHappenedIsAskedOfItsOwner(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(path string, file *ast.File) bool { return len(rawOccurredAtCompares(path, file)) > 0 },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	seen := 0
	for _, src := range scope.Files(t) {
		if strings.HasPrefix(filepath.Clean(src.Path), occurredAtOwner) {
			continue
		}
		found := rawOccurredAtCompares(src.Path, src.File)
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
		case !compare.waived:
			t.Errorf("%s compares an activity's occurred_at with a moment raw. A day entry is stored at its "+
				"date's noon UTC, so ask occurredday.BegunSQL, MeetingTookPlaceSQL, HappenedSQL or AheadSQL. "+
				"If it is a window, an age or a keyset bound, say so on its line: -- occurred_at raw: <reason>", compare.where)
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

// ordering is one ordering comparison of some row's occurred_at on a line,
// read with the column on the operator's left.
type ordering struct {
	alias string
	op    string
	// againstClock says the other side is the statement's own clock.
	againstClock bool
}

// orderingsIn reads every ordering comparison of occurred_at on one line.
func orderingsIn(line string) []ordering {
	var out []ordering
	for _, at := range occurredAtColumn.FindAllStringSubmatchIndex(line, -1) {
		alias := ""
		for _, group := range []int{2, 4, 6} {
			if at[group] >= 0 {
				alias = strings.ToLower(line[at[group]:at[group+1]])
			}
		}
		if after := orderAfter.FindStringSubmatchIndex(line[at[1]:]); after != nil {
			op := line[at[1]+after[2] : at[1]+after[3]]
			if op != "<>" && op != "!=" {
				out = append(out, ordering{alias: alias, op: op, againstClock: clockFirst.MatchString(line[at[1]+after[1]:])})
			}
			continue
		}
		if before := orderBefore.FindStringSubmatchIndex(line[:at[0]]); before != nil {
			op := line[before[2]:before[3]]
			out = append(out, ordering{alias: alias, op: flipped(op), againstClock: clockLast.MatchString(line[:before[2]])})
		}
	}
	return out
}

// asksWhetherHappened says an ordering reads as "has it happened by then".
// A `<` is a period's upper edge when its statements also bound the column below.
func asksWhetherHappened(found ordering, statements []string) bool {
	switch found.op {
	case "<=", ">":
		return true
	case ">=":
		return found.againstClock
	}
	for _, statement := range statements {
		for _, line := range strings.Split(statement, "\n") {
			for _, other := range orderingsIn(line) {
				if other.alias == found.alias && other.op == ">=" {
					return false
				}
			}
		}
	}
	return true
}

// linesComparing judges each line of text and returns the raw comparisons on
// activity rows, scopes naming the tables nearest first.
func linesComparing(where, text string, statements []string, scopes ...tables) []rawCompare {
	var out []rawCompare
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		holding := statementsHolding(line, statements)
		for _, found := range orderingsIn(line) {
			if !comparesActivity(found.alias, scopes...) || !asksWhetherHappened(found, holding) {
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

// statementsHolding are the statements a line is part of.
func statementsHolding(line string, statements []string) []string {
	var out []string
	if trimmed := strings.TrimSpace(line); trimmed != "" {
		for _, statement := range statements {
			if strings.Contains(statement, trimmed) {
				out = append(out, statement)
			}
		}
	}
	return out
}

// rawOccurredAtCompares finds each raw as-of comparison of an activity row's
// occurred_at in file's string literals.
func rawOccurredAtCompares(path string, file *ast.File) []rawCompare {
	var out []rawCompare
	whole := tablesIn(gatekit.SQLStatementsOf(file)...)
	for _, decl := range file.Decls {
		statements := gatekit.SQLStatementsOf(decl)
		declared := tablesIn(statements...)
		ast.Inspect(decl, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, isString := gatekit.LiteralText(lit)
			if !isString {
				return true
			}
			out = append(out, linesComparing(path, text, append([]string{text}, statements...),
				tablesIn(text), declared, whole)...)
			return true
		})
	}
	return out
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

type sqlBody struct{ where, text string }

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
		for _, body := range liveDefinitions(files) {
			if len(linesComparing(body.where, body.text, []string{body.text}, tablesIn(body.text))) > 0 {
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
// migrations leave in the schema, read in version order.
func liveDefinitions(files []sqlFile) []sqlBody {
	slices.SortFunc(files, func(a, b sqlFile) int { return strings.Compare(a.path, b.path) })
	live := map[string]sqlBody{}
	for _, file := range files {
		for _, at := range sqlDefinition.FindAllStringSubmatchIndex(file.text, -1) {
			name := strings.TrimPrefix(strings.ToLower(strings.ReplaceAll(file.text[at[6]:at[7]], `"`, "")), "public.")
			if strings.EqualFold(file.text[at[2]:at[3]], "DROP") {
				delete(live, name)
				continue
			}
			live[name] = sqlBody{where: file.path + " " + name, text: definitionBody(file.text[at[1]:], file.text[at[4]:at[5]])}
		}
	}
	out := make([]sqlBody, 0, len(live))
	for _, body := range live {
		out = append(out, body)
	}
	return out
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
	if end := strings.Index(rest, ";"); end >= 0 {
		return rest[:end]
	}
	return rest
}

// bodyCompares judges a body once the owner's SQL rendered in it is taken out.
func bodyCompares(body sqlBody) []rawCompare {
	bound := tablesIn(body.text)
	text := body.text
	for alias, table := range bound.aliases {
		if table != "activity" {
			continue
		}
		for _, owned := range []string{
			occurredday.HappenedSQL(alias, "now()"), occurredday.MeetingTookPlaceSQL(alias, "now()"),
			occurredday.BegunSQL(alias, "now()"), occurredday.AheadSQL(alias, "now()"),
			occurredday.EndSQL(alias), occurredday.StartSQL(alias),
		} {
			text = strings.ReplaceAll(text, owned, " ")
		}
	}
	return linesComparing(body.where, text, []string{text}, bound)
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
		for _, body := range liveDefinitions(c.files) {
			got += len(bodyCompares(body))
		}
		if got != c.want {
			t.Errorf("%s: the census found %d raw comparisons, want %d", name, got, c.want)
		}
	}
}

// The census sees each as-of spelling, reads past other tables, and holds a
// waiver to its own line.
func TestTheOccurredAtCensusSeesEachRawComparison(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{
		"now on the right":       "SELECT 1 FROM activity a WHERE a.occurred_at <= now()",
		"ahead of a bound":       "SELECT 1 FROM activity a WHERE a.occurred_at > $2",
		"sprintf placeholder":    "SELECT 1 FROM activity a WHERE a.occurred_at <= $%[3]d",
		"bound on the left":      "SELECT 1 FROM activity m WHERE $4::timestamptz >= m.occurred_at",
		"bare column":            "SELECT 1 FROM activity WHERE occurred_at < now()",
		"fragment of no table":   "AND x.occurred_at <= $%d",
		"bound joined from Go":   "SELECT 1 FROM activity a WHERE a.occurred_at <= ",
		"ahead joined from Go":   "SELECT 1 FROM activity a WHERE a.occurred_at > ",
		"pgx named argument":     "SELECT 1 FROM activity a WHERE a.occurred_at <= @as_of",
		"coalesce bound":         "SELECT 1 FROM activity a WHERE a.occurred_at <= coalesce($1, now())",
		"greatest bound":         "SELECT 1 FROM activity a WHERE a.occurred_at > greatest($1, $2)",
		"another column":         "SELECT 1 FROM activity a JOIN deal d ON a.occurred_at <= d.closed_at",
		"column on the left":     "SELECT 1 FROM activity a JOIN deal d ON d.closed_at >= a.occurred_at",
		"before a bound":         "SELECT 1 FROM activity a WHERE a.occurred_at < $1",
		"before a named arg":     "SELECT 1 FROM activity a WHERE a.occurred_at < @before",
		"before another column":  "SELECT 1 FROM activity a JOIN deal d ON a.occurred_at < d.closed_at",
		"latest of them":         "SELECT 1 FROM activity a GROUP BY a.id HAVING max(a.occurred_at) <= $1",
		"first in a coalesce":    "SELECT 1 FROM activity a WHERE coalesce(a.occurred_at, a.created_at) <= $1",
		"cast to a date":         "SELECT 1 FROM activity a WHERE a.occurred_at::date <= current_date",
		"from now on":            "SELECT 1 FROM activity a WHERE a.occurred_at >= now()",
		"another alias's period": "SELECT 1 FROM activity a JOIN activity b ON b.id = a.id WHERE b.occurred_at >= $1 AND a.occurred_at < $2",
	} {
		if got := rawOccurredAtCompares("seeded.go", parseSeed(t, sql)); len(got) != 1 || got[0].waived {
			t.Errorf("%s: the census found %+v in %q, want one unwaived comparison", name, got, sql)
		}
	}
	for name, sql := range map[string]string{
		"another table":      "SELECT 1 FROM audit_log au WHERE au.occurred_at <= $1",
		"another bare table": "DELETE FROM capture_trace WHERE occurred_at < now() - $1::interval",
		"a period's edges":   "SELECT 1 FROM activity a WHERE a.occurred_at >= $1 AND a.occurred_at < $2",
		"a lower edge":       "SELECT 1 FROM activity a WHERE a.occurred_at >= $1",
		"not equal":          "SELECT 1 FROM activity a WHERE a.occurred_at <> now()",
		"a keyset row":       "SELECT 1 FROM activity a WHERE (a.occurred_at, a.id) > ($1, $2)",
		"a json arrow":       "SELECT a.after->>'x' FROM activity a WHERE a.after->>'at' = a.occurred_at::text",
	} {
		if got := rawOccurredAtCompares("seeded.go", parseSeed(t, sql)); len(got) != 0 {
			t.Errorf("%s: the census flagged %+v in %q", name, got, sql)
		}
	}
	joined := "package p\nfunc f(b string) string { return `SELECT 1 FROM activity a WHERE a.occurred_at >= ` + b + ` AND a.occurred_at < ` + b }\n"
	if got := rawOccurredAtCompares("seeded.go", parseSource(t, joined)); len(got) != 0 {
		t.Errorf("a period whose edges Go joins on was flagged: %+v", got)
	}
	fragment := "package p\nconst q = `SELECT 1 FROM audit_log a WHERE `\nfunc f() string { return \" AND a.occurred_at <= \" }\n"
	if got := rawOccurredAtCompares("seeded.go", parseSource(t, fragment)); len(got) != 0 {
		t.Errorf("a fragment whose file binds its alias to audit_log was flagged: %+v", got)
	}
	waived := "SELECT 1 FROM activity a\n WHERE a.occurred_at > $1 -- occurred_at raw: the window's lower edge\n AND a.occurred_at <= now()"
	got := rawOccurredAtCompares("seeded.go", parseSeed(t, waived))
	if len(got) != 2 || !got[0].waived || got[1].waived {
		t.Errorf("a waiver reached past its own line or missed it: %+v", got)
	}
	last := "SELECT 1 FROM activity a WHERE a.occurred_at > $1 -- occurred_at raw: the window has a lower edge"
	if got := rawOccurredAtCompares("seeded.go", parseSeed(t, last)); len(got) != 1 || !got[0].swallows {
		t.Errorf("a waiver ending its string passed: %+v", got)
	}
}

func parseSeed(t *testing.T, sql string) *ast.File {
	t.Helper()
	return parseSource(t, "package p\nconst q = `"+sql+"`\n")
}

func parseSource(t *testing.T, source string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "seeded.go", source, 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
