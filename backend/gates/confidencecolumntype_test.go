// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every confidence column is numeric(4,3).
//
// Fifteen columns hold a machine's confidence in a derived fact, and six types
// held them: `real`, `double precision`, bare `numeric`, and three different
// precisions. The values are compared against thresholds and shown on screen,
// so one of them reading back as 0.8499999 is a report nobody can reproduce.
//
// `real` and `double precision` cannot hold 0.85 at all. Bare `numeric`
// accepts any scale, so two writers of one column can disagree about how much
// precision it keeps.
//
// The corpus derives from the head catalog, so a sixteenth column arrives
// failing until it carries the type or a waiver.
//
// The 0 to 1 range is a separate rule, held for score-like columns by
// scorecolumnbounds_test.go. Adding `confidence` to that corpus is the way to
// cover it here, rather than a second reader of what a range CHECK looks like.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// confidenceColumn reads a catalog column line for a confidence column, with
// its declared type. A CHECK line names a constraint and is skipped.
var confidenceColumn = regexp.MustCompile(`^public\.(\w+)\.(\w*confidence\w*) (.+)$`)

// confidenceType is the one representation: three decimal places, exact.
const confidenceType = "numeric(4,3)"

// notAConfidenceScore records each confidence-named column that holds something
// else, with why. It is what makes a new column fail: an unlisted one is
// neither the standard type nor a stated exception.
var notAConfidenceScore = gatekit.Waive(map[string]string{})

// confidenceColumns derives the corpus, mapping "table.column" to its type.
func confidenceColumns(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	columns := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, " CHECK (") {
			continue
		}
		m := confidenceColumn.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		columns[m[1]+"."+m[2]] = strings.TrimSpace(
			regexp.MustCompile(`\s+(?:NOT NULL|gen=|def=).*$`).ReplaceAllString(m[3], ""))
	}
	return columns
}

func TestEveryConfidenceColumnHasOneType(t *testing.T) {
	t.Parallel()
	defer notAConfidenceScore.AssertAllMatched(t)
	columns := confidenceColumns(t)
	if len(columns) < 14 {
		t.Fatalf("the catalog yields %d confidence columns; this gate would judge almost "+
			"nothing, so the reader has stopped matching the catalog's line shape", len(columns))
	}
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if columns[name] == confidenceType || notAConfidenceScore.Waived(t, name) {
			continue
		}
		t.Errorf("%s is %s, and a confidence is %s: exact to three decimals, which neither a "+
			"float type nor an unconstrained numeric gives. Alter it in a migration, or waive "+
			"it in notAConfidenceScore with what it holds instead.", name, columns[name], confidenceType)
	}
}

// The reader is held to the shapes it must see. The corpus is a name pattern,
// and a pattern that stops matching reports a clean census.
func TestTheConfidenceReaderSeesEachShape(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		line string
		want bool
	}{
		{"public.ai_call.decision_confidence numeric(4,3) gen=- def=-", true},
		{"public.signal.resolution_confidence numeric(4,3) NOT NULL gen=- def=-", true},
		{"public.company_fact.confidence real gen=- def=-", true},
		{"public.lead.score smallint NOT NULL gen=- def=-", false},
		{"public.ai_call.ai_call_decision_confidence_range CHECK ((decision_confidence >= 0))", false},
	} {
		got := confidenceColumn.MatchString(c.line) && !strings.Contains(c.line, " CHECK (")
		if got != c.want {
			t.Errorf("the confidence reader matched %q = %v, want %v", c.line, got, c.want)
		}
	}
}

// The type is read off the line rather than guessed. A column whose line gains
// a NOT NULL or a default is still read as its type.
func TestTheConfidenceTypeIsReadFromTheLine(t *testing.T) {
	t.Parallel()
	columns := confidenceColumns(t)
	for name, declared := range columns {
		if strings.ContainsAny(declared, "=") || strings.Contains(declared, "NOT NULL") {
			t.Errorf("%s read its type as %q, which still carries the line's trailing fields",
				name, declared)
		}
	}
}
