// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// A bigint that reaches a JSON client is bounded to what the client can hold.
//
// Postgres bigint runs from −2^63 to 2^63−1. A JavaScript number represents integers
// exactly only to ±(2^53−1), and the contract sends the first as the second: a field declared
// `format: int64` becomes a plain `number` in frontend/src/api/schema.d.ts. Above the
// line JSON.parse rounds to the nearest double — silently, with no error anywhere and
// nothing in the generated types marking the boundary. The mapping is correct on part
// of its domain and wrong on the rest.
//
// Today's values do not reach 2^53, and that is a property of the DATA rather than of
// the mapping: nothing states it, tests it or enforces it. The margin is also thinner
// than the round number suggests — the zero-decimal currencies in
// currency_minor_digits cost two orders of magnitude, so the ceiling moved 100x
// because of a row in a lookup table and nothing noticed.
//
// The rule: a bigint whose value reaches a JSON client is either bounded to the
// exactly representable range, or is not sent as a JSON number. Never neither.
//
// BOTH HALVES DERIVE — the columns from the committed catalog, the published names
// from crm.yaml — so the rule cannot fall short as columns and fields are added. That
// is the point of this gate: the constraints are mechanical, and what was missing was
// anything that fails when the next unbounded column arrives.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// jsSafeInteger is the largest integer a JavaScript number holds exactly: 2^53−1.
const jsSafeInteger = "9007199254740991"

var (
	// Digits are part of an identifier — `amount_2` is a column somebody may add,
	// and the contract reader below already accepts them in a field name. Excluding
	// them here would skip such a column silently, with the floor still satisfied
	// by every other one.
	bigintColumn = regexp.MustCompile(`^public\.(\w+)\.(\w+) bigint\b`)
	catalogCheck = regexp.MustCompile(`^public\.(\w+)\.(\w+) CHECK \((.*)\)$`)
	// A property declared int64 inline: `amount_minor: { type: integer, format: int64 }`.
	inlineInt64 = regexp.MustCompile(`^\s*([a-z0-9_]+):\s*\{[^}]*format:\s*int64`)
	// A property key on its own line, which a block form's `format: int64` sits under.
	propertyKey = regexp.MustCompile(`^(\s*)([a-z0-9_]+):\s*$`)
)

// sentAsString names the columns the contract deliberately publishes as a string
// rather than a number, which satisfies the rule the other way.
//
// EMPTY, and that is the point: no column needs the full int64 range today, so
// reaching for the string form is a decision somebody makes rather than something
// this gate infers. Inferring it from the contract was the first spelling here and it
// was unsound in the permissive direction — `version` is declared a string somewhere
// in crm.yaml and is also the name of 88 bigint columns, so a name-collision
// exempted every one of them from a rule they all needed.
//
// An entry is `table.column`, and it owes a reason: the field is a string in the
// contract, so a client doing arithmetic on it has to parse it first.
//
// Through gatekit.Waive, so an entry that stops describing a real column is reported
// rather than sitting here forever — an exemption nobody needs is an exemption
// nobody notices.
var sentAsString = gatekit.Waive(map[string]string{})

func TestABigintReachingAJSONClientIsBoundedToWhatItCanHold(t *testing.T) {
	t.Parallel()
	root := moduleRoot(t)
	published := publishedIntegerFields(t, root)
	columns, bounded := bigintColumnsAndTheirBounds(t, root)

	var checked int
	for column, tables := range columns {
		if !published[column] {
			continue
		}
		for _, table := range tables {
			// Published as a string, so no client reads it as a number.
			if sentAsString.Waived(t, table+"."+column) {
				continue
			}
			checked++
			if bounded[table+"."+column] {
				continue
			}
			t.Errorf("%s.%s is bigint and published as format: int64, with no bound to the "+
				"exactly representable range. Above %s a JavaScript client rounds the value "+
				"and nothing reports it — add CHECK (%s BETWEEN -%s AND %s), or declare the "+
				"contract field a string so it is not sent as a number",
				table, column, jsSafeInteger, column, jsSafeInteger, jsSafeInteger)
		}
	}
	// Both halves must have read something: a derivation that stopped matching would
	// check nothing and pass, which is the one way this gate must not break.
	if len(published) == 0 {
		t.Error("no contract field declares format: int64 — crm.yaml carries many, so the " +
			"reading of published fields has stopped matching")
	}
	if checked == 0 {
		t.Error("no bigint column is published as format: int64 — the two derivations no " +
			"longer meet, so this gate compares nothing")
	}
	sentAsString.AssertAllMatched(t)
}

// publishedIntegerFields reads the field names the contract sends as JSON integers,
// and the ones it sends as strings.
func publishedIntegerFields(t *testing.T, root string) map[string]bool {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(filepath.Dir(root), "backend", "api", "crm.yaml"))
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	int64Fields := map[string]bool{}
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if found := inlineInt64.FindStringSubmatch(line); found != nil {
			int64Fields[found[1]] = true
			continue
		}
		if !strings.Contains(line, "format: int64") {
			continue
		}
		// Block form: the property is the nearest key above it at a lower indent.
		if name, ok := enclosingProperty(lines, i); ok {
			int64Fields[name] = true
		}
	}
	return int64Fields
}

// enclosingProperty answers which property a `format: int64` line sits under.
//
// Bounded by INDENTATION rather than by a line count. A property whose schema runs
// long — a description block, an enum, a nested object — puts its format further from
// its key than any fixed budget guesses, and a budget that falls short returns no
// field at all: the column goes unbounded and the floor stays satisfied by every
// other one. The walk stops when the indentation leaves the property's own block,
// which is where the answer stops being there.
func enclosingProperty(lines []string, at int) (string, bool) {
	indent := len(lines[at]) - len(strings.TrimLeft(lines[at], " "))
	for i := at - 1; i >= 0; i-- {
		line := lines[i]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if found := propertyKey.FindStringSubmatch(line); found != nil && len(found[1]) < indent {
			return found[2], true
		}
		// Left the block this format belongs to: a key at or above this line's own
		// indentation is a sibling or an ancestor, not this property.
		if here := len(line) - len(strings.TrimLeft(line, " ")); here < indent && propertyKey.MatchString(line) {
			return "", false
		}
	}
	return "", false
}

// bigintColumnsAndTheirBounds reads the catalog for bigint columns and which of them
// a CHECK already bounds to the safe range.
func bigintColumnsAndTheirBounds(t *testing.T, root string) (columns map[string][]string, bounded map[string]bool) {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(root, "migrations", "testdata", "head_catalog.txt"))
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	columns, bounded = map[string][]string{}, map[string]bool{}
	for line := range strings.SplitSeq(string(body), "\n") {
		line = strings.TrimSpace(line)
		if found := bigintColumn.FindStringSubmatch(line); found != nil {
			columns[found[2]] = append(columns[found[2]], found[1])
			continue
		}
		found := catalogCheck.FindStringSubmatch(line)
		if found == nil {
			continue
		}
		// BOTH SIDES, for the SAME column. A check carrying only the upper limit
		// leaves every negative value unbounded while reading, to a reader and to
		// an earlier version of this gate, exactly like a bound — and the negative
		// literal contains the positive one as a substring, so "mentions the
		// number" cannot tell the two apart.
		for _, column := range strings.FieldsFunc(found[3], notIdentifierRune) {
			if boundsBothSides(found[3], column) {
				bounded[found[1]+"."+column] = true
			}
		}
	}
	if len(columns) == 0 {
		t.Fatal("no bigint column in the head catalog — the tree carries many, so this " +
			"reading has stopped matching the catalog's format")
	}
	return columns, bounded
}

// notIdentifierRune splits a CHECK's text into the identifiers it names.
func notIdentifierRune(r rune) bool {
	switch {
	case r == '_', r >= 'a' && r <= 'z', r >= '0' && r <= '9':
		return false
	default:
		return true
	}
}

// boundsBothSides reports whether a CHECK bounds this column above AND below at the
// safe limit.
//
// Matched per side with the column named in the comparison, so a check bounding one
// column above and a different column below does not read as bounding either.
func boundsBothSides(check, column string) bool {
	// A check joining its comparisons with OR bounds nothing — `col >= -X OR col <= X`
	// is true of every value — and no safe-range bound needs one. Anything carrying
	// OR is reported unbounded, which is the direction that fails loudly.
	if strings.Contains(strings.ToUpper(check), " OR ") {
		return false
	}
	lower := regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b\s*>=?\s*'?-` + jsSafeInteger + `\D`)
	upper := regexp.MustCompile(`\b` + regexp.QuoteMeta(column) + `\b\s*<=?\s*'?` + jsSafeInteger + `\D`)
	return lower.MatchString(check) && upper.MatchString(check)
}

// TestWhatCountsAsABoundIsProvedHere probes the bound reading directly.
//
// The walk above is satisfied by whichever columns it does read, so a reading that
// accepts a wrong bound — or misses a right one — passes with nothing to show. These
// cases are the evidence that it cannot.
func TestWhatCountsAsABoundIsProvedHere(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		check string
		bound bool
	}{{
		name:  "the shape this change writes",
		check: "((amount_minor >= '-9007199254740991'::bigint) AND (amount_minor <= '9007199254740991'::bigint))",
		bound: true,
	}, {
		name:  "upper only, leaving every negative value unbounded",
		check: "(amount_minor <= '9007199254740991'::bigint)",
		bound: false,
	}, {
		name:  "lower only",
		check: "(amount_minor >= '-9007199254740991'::bigint)",
		bound: false,
	}, {
		// Ten times the safe limit, and the safe literal is its prefix: an
		// unanchored reading calls this bounded.
		name:  "a limit ten times too large",
		check: "((amount_minor >= '-90071992547409910'::bigint) AND (amount_minor <= '90071992547409910'::bigint))",
		bound: false,
	}, {
		// True of every value, so it bounds nothing.
		name:  "both limits joined by OR",
		check: "((amount_minor >= '-9007199254740991'::bigint) OR (amount_minor <= '9007199254740991'::bigint))",
		bound: false,
	}, {
		// One column bounded above, another below, bounds neither.
		name:  "the two sides naming different columns",
		check: "((other >= '-9007199254740991'::bigint) AND (amount_minor <= '9007199254740991'::bigint))",
		bound: false,
	}} {
		t.Run(c.name, func(t *testing.T) {
			if got := boundsBothSides(c.check, "amount_minor"); got != c.bound {
				t.Errorf("boundsBothSides = %v, want %v: %s", got, c.bound, c.check)
			}
		})
	}
}

// TestACatalogColumnWithADigitIsRead holds that an identifier carrying a digit is not
// skipped — a column the reader cannot see is a column the rule does not reach, and
// the walk stays satisfied by every other one.
func TestACatalogColumnWithADigitIsRead(t *testing.T) {
	t.Parallel()
	found := bigintColumn.FindStringSubmatch("public.deal_2.amount_2 bigint NOT NULL gen=- def=-")
	if found == nil {
		t.Fatal("a bigint column whose table and column names carry digits was not read at all")
	}
	if found[1] != "deal_2" || found[2] != "amount_2" {
		t.Errorf("read table %q column %q, want deal_2 and amount_2", found[1], found[2])
	}
}
