// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package search

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	ctx := teamReaderFor("company")

	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }

	// The same split, in the same order, as Search performs.
	head, tail := splitTypedQuery("acme ein")
	headPos, tailPos := arg(head), arg(tail)
	identifierPos := bindIdentifier("acme ein", arg)

	branches, err := admittedBranchSQL(ctx, []string{"company"}, headPos, tailPos, true, identifierPos, arg)
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

// A branch with no identifier table binds no identifier parameter, because an
// unused bound parameter fails the whole statement.
func TestABranchWithNoIdentifierBindsNone(t *testing.T) {
	t.Parallel()
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	headPos, tailPos := arg(""), arg("anna@example.com")

	branches, err := admittedBranchSQL(teamReaderFor("deal"), []string{"deal"}, headPos, tailPos, true,
		bindIdentifier("anna@example.com", arg), arg)
	if err != nil {
		t.Fatalf("building the branch SQL: %v", err)
	}
	if len(branches) != 1 || len(bound) != 2 {
		t.Fatalf("got %d branch(es) and %d bound parameter(s), want 1 and 2", len(branches), len(bound))
	}
}

// The reader-facing half of the same rule, and the reason it matters: the query
// Search hands Postgres has to name every parameter Search bound. This asserts
// the SQL text itself carries both halves of the split — the finished words and
// the fragment — because a branch that dropped one would still build valid SQL
// and quietly stop matching.
func TestTheBranchSQLCarriesBothHalvesOfTheQuery(t *testing.T) {
	t.Parallel()
	ctx := teamReaderFor("company")
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }
	headPos, tailPos := arg("acme"), arg("ein")

	branches, err := admittedBranchSQL(ctx, []string{"company"}, headPos, tailPos, true, bindIdentifier("acme ein", arg), arg)
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
	ctx := teamReaderFor("company")
	var bound []any
	arg := func(v any) int { bound = append(bound, v); return len(bound) }

	head, tail := splitTypedQuery(`"automation world"`)
	if tail != "" {
		t.Fatalf("a quoted phrase left a fragment %q", tail)
	}
	headPos := arg(head)

	branches, err := admittedBranchSQL(ctx, []string{"company"}, headPos, 0, false, bindIdentifier(head, arg), arg)
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

// Mail matches the fragment as a whole word, in every parse its index speaks;
// records keep the prefix.
//
// A short prefix matches most mail, so offering it as one spent the whole query
// budget before a hit was ranked. The stemmed parses still apply, so "Vertrag"
// reaches "Verträge" whether or not the reader has typed the trailing space.
func TestMailMatchesTheFragmentWholeAndRecordsByPrefix(t *testing.T) {
	t.Parallel()
	branchSQL := func(entity string) (sql string, tailPos int) {
		ctx := teamReaderFor(entity)
		var bound []any
		arg := func(v any) int { bound = append(bound, v); return len(bound) }
		headPos, tailPos := arg(""), arg("study")
		branches, err := admittedBranchSQL(ctx, []string{entity}, headPos, tailPos, true, bindIdentifier("study", arg), arg)
		if err != nil {
			t.Fatalf("building the %s branch SQL: %v", entity, err)
		}
		return strings.Join(branches, " "), tailPos
	}

	mail, tailPos := branchSQL("activity")
	if strings.Contains(mail, "':*'") {
		t.Fatalf("mail is offered the fragment as a prefix:\n%s", mail)
	}
	// The FRAGMENT'S parameter has to appear inside a stemmed arm. Counting
	// occurrences of the stemmed parse is not enough: the finished-words half of
	// an activity branch already carries them.
	for _, config := range []string{"german", "english"} {
		stemmedFragment := fmt.Sprintf("websearch_to_tsquery('%s', f_unaccent($%d))", config, tailPos)
		if !strings.Contains(mail, stemmedFragment) {
			t.Fatalf("the fragment does not carry the %s parse, so a single-word search loses stemming", config)
		}
	}
	if contacts, _ := branchSQL("contact"); !strings.Contains(contacts, "':*'") {
		t.Fatalf("a record no longer matches the fragment as a prefix:\n%s", contacts)
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

// The match parses the fragment only as a prefix, so a whole-word parse of it
// in the score can only be the lift that puts Philip above every Philipp.
func TestTheScoreLiftsTheFragmentAsAWholeWord(t *testing.T) {
	t.Parallel()
	wholeFragment := "websearch_to_tsquery('simple', f_unaccent($2))"
	if score := scoreExpression(entityContact, "t", 1, 2, true); !strings.Contains(score, wholeFragment) {
		t.Fatalf("the score carries no whole-word parse of the fragment, so Philip ties with every Philipp: %s", score)
	}
	if score := scoreExpression(entityContact, "t", 1, 0, false); strings.Contains(score, "::int") {
		t.Fatalf("a query with no fragment is scored for one: %s", score)
	}
	bonus, _, _ := strings.Cut(scoreExpression(entityActivity, "t", 1, 2, true), "::int")
	if strings.Contains(bonus, "english") || strings.Contains(bonus, "german") {
		t.Fatalf("the activity bonus is stemmed, so \"studies\" earns it for \"study\": %s", bonus)
	}
}

// The activity branch ranks a bounded sample of its visible matches, and the
// bound is the named constant; a record branch ranks every match.
func TestMailRanksABoundedSampleAndRecordsRankEveryMatch(t *testing.T) {
	t.Parallel()
	for entity, wantSample := range map[string]bool{"activity": true, "contact": false} {
		var bound []any
		arg := func(v any) int { bound = append(bound, v); return len(bound) }
		headPos := arg("contract")
		branches, err := admittedBranchSQL(teamReaderFor(entity), []string{entity}, headPos, 0, false, bindIdentifier("contract", arg), arg)
		if err != nil {
			t.Fatalf("building the %s branch SQL: %v", entity, err)
		}
		sql := strings.Join(branches, " ")
		sampled := false
		for pos, v := range bound {
			if n, ok := v.(int); ok && n == activityRankSample && strings.Contains(sql, fmt.Sprintf("LIMIT $%d)", pos+1)) {
				sampled = true
			}
		}
		if sampled != wantSample {
			t.Errorf("%s branch ranks a sample of %d: %v, want %v:\n%s", entity, activityRankSample, sampled, wantSample, sql)
		}
	}
}

// A fragment hit finds the record but does not lift it into the whole-word
// tier, or an address fragment would outrank a real name match.
func TestOnlyAWholeIdentifierRaisesTheScore(t *testing.T) {
	t.Parallel()
	b := searchBranch{identifier: storekit.ContactEmails}
	pos := func() int { return 4 }

	if match := b.ownMatch("c", "q", pos); !strings.Contains(match, "LIKE") {
		t.Errorf("the match no longer finds a fragment of an address: %s", match)
	}
	if score := b.ownScore("c", "1", pos); strings.Contains(score, "LIKE") {
		t.Errorf("a fragment of an address raises the score: %s", score)
	}
}
