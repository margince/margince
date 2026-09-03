// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"
	"testing"
)

// Which part of a query is matched whole, and which is offered as a prefix.
//
// Only the fragment the reader is still typing widens. The words behind it are
// ones they finished and meant, and the two halves are AND-ed: a prefix that
// stood alone would let "zebra wor" reach a record carrying neither word, so
// typing more would widen the answer instead of narrowing it.
func TestSplitTypedQuerySeparatesFinishedWordsFromTheOneBeingTyped(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name     string
		query    string
		wantHead string
		wantTail string
	}{
		{"one unfinished word", "automa", "", "automa"},
		{"the last of several", "automation wor", "automation", "wor"},
		{
			"a trailing space means the word is finished",
			"automation ",
			"automation",
			"",
		},
		{
			"a closing quote means the reader meant those words entire",
			`"automation world"`,
			`"automation world"`,
			"",
		},
		// A quote ANYWHERE means the reader is writing a phrase, so the string
		// stays whole even when a bare word trails it. Completing that word
		// would mean reasoning about which side of the quotes it falls on, and
		// getting that wrong tears an operator off its operand.
		{
			"a bare word after a quoted phrase still does not split",
			`"key account" ber`,
			`"key account" ber`,
			"",
		},
		{"a tab separates as a space does", "automation\twor", "automation", "wor"},
		{"an accented fragment survives whole", "Müll", "", "Müll"},
		{
			"three words: only the third is a fragment",
			"acme gmbh ein",
			"acme gmbh",
			"ein",
		},
		// The websearch operators bind to the words around them, so the string
		// under that grammar must not be cut. "automation or katrin" split
		// would ask for the whole words "automation or" AND a prefix "katrin",
		// which is neither what was typed nor what was meant — and it silently
		// returned nothing.
		{
			"a disjunction is composed, not typed",
			"automation or katrin",
			"automation or katrin",
			"",
		},
		{"OR in any case", "automation OR katrin", "automation OR katrin", ""},
		{"a negated term", "automation -world", "automation -world", ""},
		{
			"a quote anywhere means a phrase is being written",
			`"automation world" x`,
			`"automation world" x`,
			"",
		},
		{"a lone word that merely contains or", "corridor", "", "corridor"},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			head, tail := splitTypedQuery(c.query)
			if head != c.wantHead || tail != c.wantTail {
				t.Fatalf("splitTypedQuery(%q) = (%q, %q), want (%q, %q)",
					c.query, head, tail, c.wantHead, c.wantTail)
			}
		})
	}
}

// Every parameter Search BINDS must be USED by the SQL it builds.
//
// Postgres infers a parameter's type from where it appears; one that is bound
// and never referenced cannot be inferred, and the whole statement fails with
// "could not determine data type of parameter $1". That is exactly what this
// change first did: the prefix arm replaced the whole-query parameter in every
// branch while Search still bound it, so every search answered 500 — and the
// frontend rendered the failure as "no results", which reads like the feature
// simply not working.
//
// The binding order lives in `Search`, not in `admittedBranchSQL`, so this
// mirrors what Search binds before its branches: a test that bound its own
// parameters would prove nothing about the caller that actually has the bug.
func TestSearchBindsNoParameterItsSQLDoesNotUse(t *testing.T) {
	t.Parallel()
	ctx := teamReaderFor("organization")

	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }

	// The same split, in the same order, as Search performs.
	head, tail := splitTypedQuery("acme ein")
	headPos, tailPos := arg(head), arg(tail)

	branches, err := admittedBranchSQL(ctx, []string{"organization"}, headPos, tailPos, true, arg)
	if err != nil {
		t.Fatalf("building the branch SQL: %v", err)
	}
	if len(branches) == 0 {
		t.Fatal("no branch was admitted, so this proves nothing about the SQL")
	}

	sql := strings.Join(branches, " ")
	for i := range bound {
		placeholder := fmt.Sprintf("$%d", i+1)
		if !strings.Contains(sql, placeholder) {
			t.Fatalf("parameter %s is bound and never used; Postgres cannot infer its type, so the statement fails at run time", placeholder)
		}
	}
}

// The reader-facing half of the same rule, and the reason it matters: the query
// Search hands Postgres has to name every parameter Search bound. This asserts
// the SQL text itself carries both halves of the split — the finished words and
// the fragment — because a branch that dropped one would still build valid SQL
// and quietly stop matching.
func TestTheBranchSQLCarriesBothHalvesOfTheQuery(t *testing.T) {
	t.Parallel()
	ctx := teamReaderFor("organization")
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	headPos, tailPos := arg("acme"), arg("ein")

	branches, err := admittedBranchSQL(ctx, []string{"organization"}, headPos, tailPos, true, arg)
	if err != nil {
		t.Fatalf("building the branch SQL: %v", err)
	}
	sql := strings.Join(branches, " ")

	if !strings.Contains(sql, "websearch_to_tsquery") {
		t.Fatal("the finished words are no longer matched whole")
	}
	if !strings.Contains(sql, "':*'") {
		t.Fatal("the fragment is no longer offered as a prefix")
	}
	// AND, not OR: OR-ed, the prefix alone satisfies the match and typing more
	// words widens the answer instead of narrowing it.
	if !strings.Contains(sql, "&&") {
		t.Fatal("the two halves are not AND-ed, so a prefix can match on its own")
	}
}

// A finished query gets NO prefix arm, and that is not an optimisation.
//
// `quote_literal("")` renders `”`, and `”:*` is a tsquery syntax error, so an
// arm built for an absent fragment answered 500 on every finished query — a
// quoted phrase, a trailing space, an operator query. The arm is omitted
// instead, which also makes those queries match exactly what they matched
// before this change.
func TestAQueryWithNoFragmentGetsNoPrefixArm(t *testing.T) {
	t.Parallel()
	ctx := teamReaderFor("organization")
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }

	head, tail := splitTypedQuery(`"automation world"`)
	if tail != "" {
		t.Fatalf("a quoted phrase left a fragment %q", tail)
	}
	headPos := arg(head)

	branches, err := admittedBranchSQL(ctx, []string{"organization"}, headPos, 0, false, arg)
	if err != nil {
		t.Fatalf("building the branch SQL: %v", err)
	}
	sql := strings.Join(branches, " ")

	if strings.Contains(sql, "':*'") {
		t.Fatal("a prefix arm was built for a query with no fragment; it renders '':* and raises a tsquery syntax error")
	}
	if !strings.Contains(sql, "websearch_to_tsquery") {
		t.Fatal("the finished words are no longer matched at all")
	}
}

// A trailing separator survives as far as the split.
//
// `Search` trims its input, and trimming the separator BEFORE the split is what
// makes "a finished word" unreachable: "ann " becomes "ann", which the splitter
// reads as a fragment and searches as `ann:*`, so a completed search silently
// widened to reach "annual". A unit test on the splitter alone cannot see this —
// it never runs the trim — so this asserts the shape `Search` actually produces.
func TestATrailingSpaceStillMarksTheWordFinished(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name  string
		input string
		want  string
	}{
		{"a trailing space is kept", "ann ", ""},
		{"leading space is not the reader's", "  ann", "ann"},
		{"both, and only the leading one goes", "  ann ", ""},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			// The PRODUCT'S normalisation, not a copy of it: a test that
			// trimmed for itself would pass while Search erased the separator.
			if _, tail := splitTypedQuery(normalizeQuery(c.input)); tail != c.want {
				t.Fatalf("after Search's trim, splitTypedQuery(%q) tail = %q, want %q",
					c.input, tail, c.want)
			}
		})
	}
}

// The fragment keeps the activity branch's stemming.
//
// A prefix compares `simple` lexemes while the activity index stores stems, so
// "study" as `study:*` reaches neither the stored `studi` nor the simple
// `studies`. Every search is a fragment as it is typed, so without this the
// branch's whole reason for existing — reaching "Verträge" from "Vertrag" —
// stopped applying to exactly the case a reader hits first.
func TestTheActivityFragmentKeepsItsStemming(t *testing.T) {
	t.Parallel()
	ctx := teamReaderFor("activity")
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	headPos, tailPos := arg(""), arg("study")

	branches, err := admittedBranchSQL(ctx, []string{"activity"}, headPos, tailPos, true, arg)
	if err != nil {
		t.Fatalf("building the branch SQL: %v", err)
	}
	sql := strings.Join(branches, " ")

	if !strings.Contains(sql, "':*'") {
		t.Fatal("the fragment is no longer offered as a prefix")
	}
	// The FRAGMENT'S parameter has to appear inside a stemmed arm. Counting
	// occurrences of the stemmed parse is not enough: the finished-words half of
	// an activity branch already carries them, so a fragment that dropped its
	// own still left the count satisfied.
	for _, config := range []string{"german", "english"} {
		stemmedFragment := fmt.Sprintf(
			"websearch_to_tsquery('%s', f_unaccent($%d))", config, tailPos)
		if !strings.Contains(sql, stemmedFragment) {
			t.Fatalf("the fragment does not carry the %s parse, so a single-word search loses stemming", config)
		}
	}
}

// The fragment is escaped as a tsquery, not as a SQL literal.
//
// `quote_literal` emits an E-string for text carrying a backslash, and the `E`
// is then read as a lexeme of its own: "ACME\Sales" searched for a phantom `e`.
// `plainto_tsquery` renders the same input into sanitized lexemes and never
// raises on text off a request.
func TestTheFragmentIsEscapedAsATsqueryNotASQLLiteral(t *testing.T) {
	t.Parallel()
	sql := prefixArmSQL(7)
	if strings.Contains(sql, "quote_literal") {
		t.Fatal("quote_literal quotes for SQL, not for tsquery: a backslash in the fragment becomes an E-string and the E is parsed as a lexeme")
	}
	if !strings.Contains(sql, "plainto_tsquery") {
		t.Fatal("the fragment is not sanitized into lexemes")
	}
}
