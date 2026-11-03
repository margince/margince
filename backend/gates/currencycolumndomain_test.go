// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every currency column carries the `currency_code` domain.
//
// Twenty-six columns hold a currency code. Before the domain they held two
// types and two levels of rule. Ten carried their own copy of `^[A-Z]{3}$` and
// fifteen carried nothing. A column with no rule took 'ab1' or three spaces,
// and a `text` one took any length.
//
// The domain states the rule once. This gate holds the columns to it, so a
// twenty-seventh arrives failing until it carries the domain or a waiver.
//
// What this cannot see: the head catalog records a column's type and not the
// domain's own constraint, so a change to `currency_code_shape` is invisible
// here. The migration that creates it is the only statement of the regex.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// currencyColumn reads a catalog column line for a currency column with its
// declared type. A CHECK or key line names a constraint and is skipped.
var currencyColumn = regexp.MustCompile(`^public\.(\w+)\.(\w*currency\w*) (.+)$`)

// currencyDomain is the one type, carrying the one rule.
const currencyDomain = "currency_code"

// notACurrencyCode records each currency-named column outside the domain, with
// why. It is what makes a new column fail: an unlisted one is neither held to
// the rule nor recorded as an exception to it.
var notACurrencyCode = gatekit.Waive(map[string]string{
	"brief_run.revenue_norm_currency": "NOT NULL with a default of '', so the empty string is how a " +
		"brief says it has no normalised currency, and its own CHECK admits that. Padded to " +
		"char(3) the sentinel becomes three spaces, which the domain refuses.",
})

// currencyColumns derives the corpus, mapping "table.column" to its type.
func currencyColumns(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	trailing := regexp.MustCompile(`\s+(?:NOT NULL|gen=|def=).*$`)
	columns := map[string]string{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.Contains(trimmed, " CHECK (") || strings.Contains(trimmed, " KEY (") {
			continue
		}
		m := currencyColumn.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		columns[m[1]+"."+m[2]] = strings.TrimSpace(trailing.ReplaceAllString(m[3], ""))
	}
	return columns
}

func TestEveryCurrencyColumnCarriesTheDomain(t *testing.T) {
	t.Parallel()
	defer notACurrencyCode.AssertAllMatched(t)
	columns := currencyColumns(t)
	if len(columns) < 25 {
		t.Fatalf("the catalog yields %d currency columns; this gate would judge almost "+
			"nothing, so the reader has stopped matching the catalog's line shape", len(columns))
	}
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if columns[name] == currencyDomain || notACurrencyCode.Waived(t, name) {
			continue
		}
		t.Errorf("%s is %s, and a currency code is the %s domain, which carries the shape rule "+
			"with the type. Alter it in a migration, or waive it in notACurrencyCode with what "+
			"it holds instead.", name, columns[name], currencyDomain)
	}
}

// The reader is held to the shapes it must see. The corpus is a name pattern,
// and a pattern that stops matching reports a clean census.
func TestTheCurrencyReaderSeesEachShape(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		line string
		want bool
	}{
		{"public.deal.currency currency_code gen=- def=-", true},
		{"public.fx_rate.from_currency currency_code NOT NULL gen=- def=-", true},
		{"public.brief_run.revenue_norm_currency text NOT NULL gen=- def=''::text", true},
		{"public.weekly_review.base_currency currency_code gen=- def=-", true},
		{"public.deal.amount_minor bigint gen=- def=-", false},
		{"public.currency_minor_digits.currency_minor_digits_pkey PRIMARY KEY (currency)", false},
		{"public.deal.deal_currency_check CHECK ((currency ~ '^[A-Z]{3}$'))", false},
	} {
		trimmed := c.line
		skip := strings.Contains(trimmed, " CHECK (") || strings.Contains(trimmed, " KEY (")
		if got := currencyColumn.MatchString(trimmed) && !skip; got != c.want {
			t.Errorf("the currency reader matched %q = %v, want %v", c.line, got, c.want)
		}
	}
}
