// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every language or locale column has a foreign key to the language table or
// the locale shape check.
//
// A language column with no key takes any string, such as a typo in a consent
// notice's language. A locale such as de-DE carries a shape check instead, and
// each is named below. The corpus is every catalog column named like `language`
// or `locale`.

import (
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/languages"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// localeShapeColumns are the columns that hold a locale and keep their own
// shape check, with why they are not language codes.
var localeShapeColumns = gatekit.Waive(map[string]string{
	"offer_template.locale":      "a template for de-DE and one for de-AT stay different templates, so the column holds a locale",
	"lead_privacy_notice.locale": "the notice is written for a locale such as de-DE, the same value an offer template carries",
})

// localeShape is the CHECK's pattern as the catalog prints it.
const localeShape = `'^[a-z]{2}(-[A-Z]{2})?$'`

var (
	languageColumnLine = regexp.MustCompile(`^public\.(\w+)\.(\w*(?:language|locale)\w*) (?:text|character varying)\b`)
	languageKeyLine    = regexp.MustCompile(`^public\.(\w+)\.\w+ FOREIGN KEY \((\w+)\) REFERENCES language\(code\)`)
	localeCheckLine    = regexp.MustCompile(`^public\.(\w+)\.\w+ CHECK \(.*?\b(\w+) ~ ` + regexp.QuoteMeta(localeShape))
)

// languageColumnCensus holds every language or locale column of the catalog,
// those with a key to the language table, and those with the locale shape.
type languageColumnCensus struct{ columns, keyed, shaped map[string]bool }

func readLanguageColumns(catalog string) languageColumnCensus {
	census := languageColumnCensus{map[string]bool{}, map[string]bool{}, map[string]bool{}}
	for line := range strings.SplitSeq(catalog, "\n") {
		line = strings.TrimSpace(line)
		if m := languageColumnLine.FindStringSubmatch(line); m != nil {
			census.columns[m[1]+"."+m[2]] = true
		}
		if m := languageKeyLine.FindStringSubmatch(line); m != nil {
			census.keyed[m[1]+"."+m[2]] = true
		}
		if m := localeCheckLine.FindStringSubmatch(line); m != nil {
			census.shaped[m[1]+"."+m[2]] = true
		}
	}
	return census
}

// languageColumnFindings names each column that is held by neither.
func languageColumnFindings(c languageColumnCensus, locales func(string) bool) []string {
	var out []string
	for name := range c.columns {
		switch {
		case locales(name):
			if !c.shaped[name] {
				out = append(out, name+" is a locale column and has no locale shape CHECK")
			}
		case !c.keyed[name]:
			out = append(out, name+" holds a language and has no foreign key to language(code); "+
				"add one, or list it in localeShapeColumns if it holds a locale such as de-DE")
		}
	}
	sort.Strings(out)
	return out
}

func readCatalog(t *testing.T) string {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	return string(raw)
}

func TestEveryLanguageColumnIsKeyedToTheLanguageTable(t *testing.T) {
	t.Parallel()
	defer localeShapeColumns.AssertAllMatched(t)
	census := readLanguageColumns(readCatalog(t))
	if len(census.columns) < 10 {
		t.Fatalf("the catalog yields %d language or locale columns; this gate would judge almost nothing",
			len(census.columns))
	}
	for _, finding := range languageColumnFindings(census, func(name string) bool { return localeShapeColumns.Waived(t, name) }) {
		t.Error(finding)
	}
}

// The data task reads the columns the catalog holds. A column it skipped would
// never be reported.
func TestTheLanguageReportReadsEveryHeldColumn(t *testing.T) {
	t.Parallel()
	census := readLanguageColumns(readCatalog(t))
	var held []string
	for name := range census.keyed {
		held = append(held, name)
	}
	for name := range census.shaped {
		held = append(held, name)
	}
	sort.Strings(held)
	reported := slices.Clone(languages.Checked())
	sort.Strings(reported)
	if !slices.Equal(held, reported) {
		t.Errorf("the catalog holds %v by the language table or the locale shape, and the language report reads %v", held, reported)
	}
}

// Core seeds the languages it ships. The seed is SQL and the list is Go, so this
// test holds the two together.
func TestTheLanguageTableSeedIsTheShippedLanguages(t *testing.T) {
	t.Parallel()
	seed := regexp.MustCompile(`\('([a-z]{2})', '[^']*', 'core'\)`)
	var got []string
	entries, err := gatekit.ReadDir("migrations/core")
	if err != nil {
		t.Fatalf("reading the core migrations: %v", err)
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".up.sql") {
			continue
		}
		body, err := gatekit.ReadFile("migrations/core/" + entry.Name())
		if err != nil {
			t.Fatalf("reading %s: %v", entry.Name(), err)
		}
		if !strings.Contains(string(body), "INSERT INTO language") {
			continue
		}
		for _, m := range seed.FindAllStringSubmatch(string(body), -1) {
			got = append(got, m[1])
		}
	}
	want := make([]string, len(textlang.Shipped))
	for i, lang := range textlang.Shipped {
		want[i] = string(lang)
	}
	sort.Strings(got)
	sort.Strings(want)
	if !slices.Equal(got, want) {
		t.Errorf("the migrations seed the languages %v, and the product ships %v: add a migration that seeds the new language", got, want)
	}
}

// The reader is held to the shapes it must see, and the gate to a column that
// has neither a key nor a shape.
func TestTheLanguageColumnGateFailsAColumnWithNoKey(t *testing.T) {
	t.Parallel()
	catalog := strings.Join([]string{
		"public.foo.language text gen=- def=-",
		"public.bar.language text gen=- def=-",
		"public.bar.bar_language_fkey FOREIGN KEY (language) REFERENCES language(code) ON UPDATE RESTRICT ON DELETE RESTRICT NOT VALID",
		"public.baz.locale text NOT NULL gen=- def='de-DE'::text",
		"public.qux.locale text gen=- def=-",
		"public.qux.qux_locale_shape CHECK (((locale IS NULL) OR (locale ~ '^[a-z]{2}(-[A-Z]{2})?$'::text))) NOT VALID",
	}, "\n")
	findings := languageColumnFindings(readLanguageColumns(catalog), func(name string) bool {
		return name == "baz.locale" || name == "qux.locale"
	})
	want := []string{
		"baz.locale is a locale column and has no locale shape CHECK",
		"foo.language holds a language and has no foreign key to language(code); " +
			"add one, or list it in localeShapeColumns if it holds a locale such as de-DE",
	}
	if !slices.Equal(findings, want) {
		t.Errorf("findings = %q, want %q", findings, want)
	}
}
