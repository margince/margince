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
// occurred_at compared raw with now(), or bounded by a time as `<= $N` or
// `> $N`, is a finding.
//
// Some of those are not "has it happened": a window's edge, a retention age, a
// walk's keyset bound, a clock-skew allowance. Each says so on its own line of
// SQL, as `-- occurred_at raw: <reason>`, so a second comparison added to a
// waived statement is a finding of its own.
//
// Blind spot: a half-open period's `>= $from` and `< $to` are not read, so a
// "has it happened" spelled `occurred_at < $asOf` passes.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	// boundTime is a bind placeholder, $N or the $%d a Sprintf renders.
	boundTime = `(?:\$\d+|\$%(?:\[\d+\])?d)(?:::timestamptz)?`
	// clockNow is the statement's own clock.
	clockNow = `(?:now\(\)|current_timestamp)`
	// columnRef is occurred_at, with the alias it is read under if any.
	columnRef = `(?:\b([a-z_]\w*)\.)?\boccurred_at\b`
)

var (
	// occurredAtOwner is the one package allowed to compare occurred_at raw.
	occurredAtOwner = filepath.Join("internal", "shared", "kernel", "occurredday")
	// rawOccurredAtCompare is the as-of spellings, the column on either side.
	rawOccurredAtCompare = regexp.MustCompile(`(?i)` +
		columnRef + `\s*(?:(?:<=|>)\s*(?:` + boundTime + `|` + clockNow + `)|(?:<|>=)\s*` + clockNow + `)` +
		`|(?:` + boundTime + `\s*(?:>=|<)|` + clockNow + `\s*(?:<=|>=|<|>))\s*` + columnRef)
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

// minRawOccurredAtCompares is the floor under the census: fewer comparisons on
// activity rows than this means the walk reads a smaller tree than it should.
const minRawOccurredAtCompares = 15

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
		for _, found := range rawOccurredAtCompares(src.Path, src.File) {
			seen++
			switch {
			case !found.waived:
				t.Errorf("%s compares an activity's occurred_at with a moment raw. A day entry is stored at its "+
					"date's noon UTC, so ask occurredday.BegunSQL, MeetingTookPlaceSQL, HappenedSQL or AheadSQL. "+
					"If it is a window, an age or a keyset bound, say so on its line: -- occurred_at raw: <reason>", found.where)
			case found.swallows:
				t.Errorf("%s waives on the last line of its string, so the -- comment swallows any SQL joined after it; "+
					"end the line inside the string", found.where)
			case len(strings.Fields(found.reason)) < 3:
				t.Errorf("%s waives a raw occurred_at comparison with %q; say in a few words why the date does not matter",
					found.where, found.reason)
			}
		}
	}
	if seen < minRawOccurredAtCompares {
		t.Errorf("the census found %d raw occurred_at comparisons, want at least %d: it reads a smaller tree than it should",
			seen, minRawOccurredAtCompares)
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

// comparesActivity says whether a comparison's alias names an activity row. A
// literal that names tables answers for itself, and a fragment that names
// none reads its declaration's. An alias bound to no table counts.
func comparesActivity(alias string, local, decl tables) bool {
	scope := decl
	if local.namesAny {
		scope = local
	}
	if alias == "" {
		return scope.namesActivity || !scope.namesAny
	}
	table, bound := scope.aliases[strings.ToLower(alias)]
	if !bound && local.namesAny {
		table, bound = decl.aliases[strings.ToLower(alias)]
	}
	return !bound || table == "activity"
}

// rawOccurredAtCompares finds each raw as-of comparison of an activity row's
// occurred_at in file's string literals.
func rawOccurredAtCompares(path string, file *ast.File) []rawCompare {
	var out []rawCompare
	for _, decl := range file.Decls {
		declared := tables{aliases: map[string]string{}}
		for _, statement := range gatekit.SQLStatementsOf(decl) {
			declared.read(statement)
		}
		ast.Inspect(decl, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			text, isString := gatekit.LiteralText(lit)
			if !isString {
				return true
			}
			local := tables{aliases: map[string]string{}}
			local.read(text)
			lines := strings.Split(text, "\n")
			for i, line := range lines {
				for _, match := range rawOccurredAtCompare.FindAllStringSubmatch(line, -1) {
					if !comparesActivity(match[1]+match[2], local, declared) {
						continue
					}
					found := rawCompare{where: path + ": " + strings.TrimSpace(match[0])}
					if waiver := rawOccurredAtWaiver.FindStringSubmatch(line); waiver != nil {
						found.waived, found.reason, found.swallows = true, waiver[1], i == len(lines)-1
					}
					out = append(out, found)
				}
			}
			return true
		})
	}
	return out
}

// The census sees each as-of spelling, reads past other tables, and holds a
// waiver to its own line.
func TestTheOccurredAtCensusSeesEachRawComparison(t *testing.T) {
	t.Parallel()
	for name, sql := range map[string]string{
		"now on the right":     "SELECT 1 FROM activity a WHERE a.occurred_at <= now()",
		"ahead of a bound":     "SELECT 1 FROM activity a WHERE a.occurred_at > $2",
		"sprintf placeholder":  "SELECT 1 FROM activity a WHERE a.occurred_at <= $%[3]d",
		"bound on the left":    "SELECT 1 FROM activity m WHERE $4::timestamptz >= m.occurred_at",
		"bare column":          "SELECT 1 FROM activity WHERE occurred_at < now()",
		"fragment of no table": "AND x.occurred_at <= $%d",
	} {
		if got := rawOccurredAtCompares("seeded.go", parseSeed(t, sql)); len(got) != 1 || got[0].waived {
			t.Errorf("%s: the census found %+v in %q, want one unwaived comparison", name, got, sql)
		}
	}
	for name, sql := range map[string]string{
		"another table":      "SELECT 1 FROM audit_log au WHERE au.occurred_at <= $1",
		"another bare table": "DELETE FROM capture_trace WHERE occurred_at < now() - $1::interval",
		"a period's edges":   "SELECT 1 FROM activity a WHERE a.occurred_at >= $1 AND a.occurred_at < $2",
	} {
		if got := rawOccurredAtCompares("seeded.go", parseSeed(t, sql)); len(got) != 0 {
			t.Errorf("%s: the census flagged %+v in %q", name, got, sql)
		}
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
	file, err := parser.ParseFile(token.NewFileSet(), "seeded.go", "package p\nconst q = `"+sql+"`\n", 0)
	if err != nil {
		t.Fatal(err)
	}
	return file
}
