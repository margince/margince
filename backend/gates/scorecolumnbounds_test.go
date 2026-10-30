// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every column holding a bounded quantity states the bound in the schema.
//
// A clamp in Go protects the rows that one writer writes. A `CHECK` protects
// the column, so an import and a repair script meet the same rule. Eleven
// columns state a range today and nothing held them to it.
//
// The subject derives from the head catalog. A new bounded column fails here
// until it carries a range `CHECK` or a waiver. A waiver for a column that has
// since gained one fails as well.
//
// The corpus is columns whose name carries a bounded quantity, so one named for
// something else stays invisible. The reader test below plants each shape it
// must see, rather than trusting the pattern to be complete.
//
// A sum stays outside. `lead_score_history.rounded_sum` holds the value from
// before the clamp, so a raw 100.6 stores 101 and the cap acts after that. Its
// range is open, and `leadscore.go` states that where the two steps are named.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// boundedQuantityColumn reads a catalog column line for the integer family.
// A bounded quantity is stored as a whole number here; a numeric column states
// its range through precision and scale instead.
var boundedQuantityColumn = regexp.MustCompile(
	`^public\.(\w+)\.(\w*(?:score|probability|percent|points|_rate|rate_bps)\w*) (smallint|integer|bigint)\b`)

// unboundedOnPurpose records each bounded-sounding column that carries no range,
// with why. It is what makes a new column fail: an unlisted one has neither a
// bound nor a decision behind its absence.
var unboundedOnPurpose = gatekit.Waive(map[string]string{
	"lead_manual_signal.points": "a rep's own signal, which can subtract. The scorer's own " +
		"fitLowIntentSourcePenalty is -5, so a lower bound of zero would refuse a penalty.",
})

// rangedColumns reports, per "table.column", whether a CHECK compares it with
// both a floor and a ceiling.
func rangedColumns(t *testing.T, catalog string) map[string]bool {
	t.Helper()
	ranged := map[string]bool{}
	for line := range strings.SplitSeq(catalog, "\n") {
		m := catalogCheck.FindStringSubmatch(strings.TrimSpace(line))
		if m == nil {
			continue
		}
		for _, column := range columnsStatingARange(m[3]) {
			ranged[m[1]+"."+column] = true
		}
	}
	return ranged
}

// nullGuard matches the opening a nullable column's range CHECK carries:
// `(col IS NULL) OR (the range)`. The guard is part of stating the range, not a
// weakening of it, so it is removed before the comparisons are read.
var nullGuard = regexp.MustCompile(`^\(+\s*(\w+) IS NULL\)?\s*OR\s*`)

// columnsStatingARange names every column a `CHECK` definition bounds on both
// sides.
//
// It differs from boundsBothSides, which refuses any definition carrying `OR`.
// That is right for a JSON ceiling. Here it is wrong, because a nullable column
// states its range as `(col IS NULL) OR (col >= 0 AND col <= 100)`.
//
// A mention is not a range. The pairing constraint on
// `lead_score_history.score` names the column and bounds nothing, and it made
// the census behind this gate read four bounded columns as unbounded.
func columnsStatingARange(definition string) []string {
	body := definition
	if m := nullGuard.FindStringSubmatch(body); m != nil {
		body = body[len(m[0]):]
	}
	if strings.Contains(strings.ToUpper(body), " OR ") {
		// Comparisons joined by `OR` bound nothing, since `col >= 0 OR col <= 100`
		// is true of every value. Reporting it unbounded is the direction that fails.
		return nil
	}
	var ranged []string
	seen := map[string]bool{}
	for _, column := range regexp.MustCompile(`\b[a-z][a-z0-9_]*\b`).FindAllString(body, -1) {
		if seen[column] {
			continue
		}
		_, floor := comparisonBound(body, column, ">")
		_, ceiling := comparisonBound(body, column, "<")
		if floor && ceiling {
			seen[column] = true
			ranged = append(ranged, column)
		}
	}
	return ranged
}

// boundedQuantityColumns derives the corpus from the head catalog.
func boundedQuantityColumns(t *testing.T, catalog string) []string {
	t.Helper()
	var columns []string
	for line := range strings.SplitSeq(catalog, "\n") {
		if m := boundedQuantityColumn.FindStringSubmatch(strings.TrimSpace(line)); m != nil {
			columns = append(columns, m[1]+"."+m[2])
		}
	}
	sort.Strings(columns)
	return columns
}

func headCatalog(t *testing.T) string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	return string(raw)
}

func TestEveryBoundedQuantityColumnStatesItsRange(t *testing.T) {
	t.Parallel()
	defer unboundedOnPurpose.AssertAllMatched(t)
	catalog := headCatalog(t)
	columns := boundedQuantityColumns(t, catalog)
	if len(columns) < 12 {
		t.Fatalf("the catalog yields %d bounded-quantity columns; this gate would judge almost "+
			"nothing, so the reader has stopped matching the catalog's line shape", len(columns))
	}
	ranged := rangedColumns(t, catalog)
	for _, column := range columns {
		if ranged[column] || unboundedOnPurpose.Waived(t, column) {
			continue
		}
		t.Errorf("%s holds a bounded quantity and no CHECK compares it with a floor and a "+
			"ceiling. Add one in a migration, or waive it in unboundedOnPurpose with the "+
			"reason its range is open.", column)
	}
}

func TestAWaivedColumnThatGainedABoundFails(t *testing.T) {
	t.Parallel()
	catalog := headCatalog(t)
	ranged := rangedColumns(t, catalog)
	inCorpus := map[string]bool{}
	for _, column := range boundedQuantityColumns(t, catalog) {
		inCorpus[column] = true
	}
	for _, column := range unboundedOnPurpose.Subjects() {
		if ranged[column] {
			t.Errorf("unboundedOnPurpose waives %s, which now carries a range CHECK — remove "+
				"the waiver, or this registry describes a schema that is gone", column)
		}
		if !inCorpus[column] {
			t.Errorf("unboundedOnPurpose waives %s, which the catalog holds no longer — remove "+
				"the waiver", column)
		}
	}
}

// The reader is held to the shapes it must see. The corpus is a name pattern,
// and a pattern that stops matching reports a clean census with nothing to show.
func TestTheBoundedQuantityReaderSeesEachShape(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		line string
		want bool
	}{
		{"public.lead.score smallint NOT NULL gen=- def=-", true},
		{"public.commission_entry.rate_bps integer NOT NULL gen=- def=0", true},
		{"public.lead_score_history.rounded_sum smallint NOT NULL gen=- def=-", false},
		{"public.stage.win_probability smallint NOT NULL gen=- def=-", true},
		{"public.lead.score_override_reason text gen=- def=-", false},
		{"public.deal.fx_rate_to_base numeric(20,10) gen=- def=-", false},
	} {
		if got := boundedQuantityColumn.MatchString(c.line); got != c.want {
			t.Errorf("the bounded-quantity reader matched %q = %v, want %v", c.line, got, c.want)
		}
	}
}

// A CHECK that only mentions a column states no range, which is the reading
// error this gate exists to refuse.
func TestAMentionIsNotARange(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name       string
		definition string
		want       []string
	}{
		{"a mention bounds nothing", "((override_reason IS NOT NULL) OR (score = score_computed))", nil},
		{"a range states itself", "(((score >= 0) AND (score <= 100)))", []string{"score"}},
		{"a nullable range keeps its guard", "((relevance IS NULL) OR ((relevance >= 0) AND (relevance <= 100)))", []string{"relevance"}},
		{"comparisons joined by OR bound nothing", "((score >= 0) OR (score <= 100))", nil},
		{"one side only is not a range", "((points >= 0))", nil},
	} {
		got := columnsStatingARange(c.definition)
		if len(got) != len(c.want) || (len(c.want) == 1 && got[0] != c.want[0]) {
			t.Errorf("%s: %q read as %v, want %v", c.name, c.definition, got, c.want)
		}
	}
}
