// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The docs name exactly the locales the tree ships, and the catalogs are the
// set the Go validators admit.
//
// Key parity is compile-time, so how many catalogs a new string lands in is
// something a contributor needs before pushing, and only a page says — no
// compiler reads prose. Naming too few costs a review round; naming a locale
// the tree lacks sends the reader after a file that is not there.
//
// The shipped set is DERIVED from the catalog directory, never listed here.
// languageset_test.go holds the neighbouring pair, `textlang.Shipped` against
// the contract enums; neither suite sees both registries, which is why the
// catalogs meet textlang below.

import (
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	localeCatalogDir       = "../frontend/src/i18n"
	localeArchitecturePage = "../docs/explanation/frontend-architecture.md"
	localeExtensionPage    = "../docs/how-to/add-an-extension.md"
)

var (
	// localeCode is a language code as both the catalogs and the pages spell it.
	localeCode = regexp.MustCompile(`^[a-z]{2}$`)
	// localeCatalogName matches a catalog's own filename and nothing else in
	// the directory beside it.
	localeCatalogName = regexp.MustCompile(`^([a-z]{2})\.ts$`)
	// localeCatalogPath matches a backtick span that IS a catalog file, whole:
	// `src/i18n/en.ts` and the bare `de.ts` beside it in the copy table row.
	// Anchoring both ends is what keeps `i18n.test.ts` (basename `i18n.test`)
	// and `App.tsx` out — a loose match on `xx.ts` finds a code in each.
	localeCatalogPath = regexp.MustCompile(`^(?:[\w./-]*/)?([a-z]{2})\.ts$`)
	// localeListGroup is a parenthesised run, the form the how-to writes its
	// list in: "every locale the installation ships (en, de, vi)".
	localeListGroup = regexp.MustCompile(`\(([^()]*)\)`)
)

// localeClaim is one place a page states the shipped set: the codes it names,
// the text that names them, and the line the statement starts on.
type localeClaim struct {
	line   int
	region string
	codes  map[string]bool
}

// opens reports whether this statement is the one a site asks for: a bullet, a
// table row, or anything on a page that states the set once. A non-empty opener
// also demands the catalog directory, so the bullet about routes and the table
// row about money are not mistaken for the statement that is missing.
func (c localeClaim) opens(opener, dir string) bool {
	if opener == "" {
		return true
	}
	return strings.HasPrefix(strings.TrimLeft(c.region, " "), opener) &&
		strings.Contains(c.region, dir)
}

func TestTheDocsNameEveryLocaleTheTreeShips(t *testing.T) {
	t.Parallel()
	shipped := localesShipped(t)

	// Both pages state the set, and the architecture page states it twice: a
	// contributor reaches for the catalogs through the prose or through the
	// table, and a set named in only one is a page disagreeing with itself.
	// Each site is anchored on the catalog directory as the page spells it,
	// so the anchor moves when the directory does.
	dir := localeDocPath()
	sites := []struct {
		page, opener, name string
	}{
		{localeArchitecturePage, "- ", "the `" + dir + "` bullet"},
		{localeArchitecturePage, "|", "the copy row of the where-to-look table"},
		{localeExtensionPage, "", "the sentence naming every locale the installation ships"},
	}
	for _, site := range sites {
		claims := localeClaimsIn(t, site.page)
		stated := 0
		for _, claim := range claims {
			if !claim.opens(site.opener, dir) {
				continue
			}
			stated++
			reportLocaleDrift(t, site.page, claim, shipped)
		}
		if stated == 0 {
			t.Errorf("%s no longer states the shipped locales at %s.\n"+
				"That statement is how a contributor learns which catalogs a new string has to "+
				"land in, and it is the only place it is written. Restore it, naming %s.",
				site.page, site.name, strings.Join(sortedLocales(shipped), ", "))
		}
	}
}

// The catalogs are the languages the server admits, in both directions.
//
// languageset_test.go leaves this pair to "the frontend's own suite", and that
// suite cannot import Go: a catalog the Go list refuses is a language the
// switcher offers and `PUT /v1/me/locale` rejects, and a Go language with no
// catalog is a member reading English under their own language's name.
func TestEveryShippedCatalogIsALanguageTheServerAdmits(t *testing.T) {
	t.Parallel()
	shipped := localesShipped(t)
	admitted := map[string]bool{}
	for _, lang := range textlang.Shipped {
		admitted[string(lang)] = true
	}
	if len(admitted) == 0 {
		t.Fatal("textlang.Shipped declares no language — a comparison against an empty list " +
			"agrees with every catalog, which is the one way this check must not fail")
	}
	root := strings.TrimPrefix(localeCatalogDir, "../")
	for _, code := range sortedLocales(shipped) {
		if !admitted[code] {
			t.Errorf("%s/%s.ts ships and textlang.Shipped does not admit %q.\n"+
				"The switcher offers that language and the server refuses it as a member's own. "+
				"Add it to textlang.Shipped, widening the contract enums with it, or drop the catalog.",
				root, code, code)
		}
	}
	for _, lang := range textlang.Shipped {
		if code := string(lang); !shipped[code] {
			t.Errorf("textlang.Shipped admits %q and %s/%s.ts does not exist.\n"+
				"A member who picks it reads English under their own language's name. "+
				"Add the catalog, or drop the code from textlang.Shipped.", code, root, code)
		}
	}
}

// localesShipped reads the catalog directory and returns the codes it carries.
func localesShipped(t *testing.T) map[string]bool {
	t.Helper()
	entries, err := os.ReadDir(localeCatalogDir)
	if err != nil {
		t.Fatalf("reading the catalog directory %s: %v", localeCatalogDir, err)
	}
	shipped := map[string]bool{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if named := localeCatalogName.FindStringSubmatch(entry.Name()); named != nil {
			shipped[named[1]] = true
		}
	}
	if len(shipped) == 0 {
		t.Fatalf("no catalog found under %s — a gate reading an empty set agrees with every page, "+
			"which is the one way this check must not fail", localeCatalogDir)
	}
	return shipped
}

// localeDocPath is how a page under docs/ spells the catalog directory: both
// pages write frontend paths relative to `frontend/`.
func localeDocPath() string {
	return strings.TrimPrefix(filepath.ToSlash(localeCatalogDir), "../frontend/") + "/"
}

// reportLocaleDrift compares one statement against the tree in both directions.
func reportLocaleDrift(t *testing.T, page string, claim localeClaim, shipped map[string]bool) {
	t.Helper()
	root := strings.TrimPrefix(localeCatalogDir, "../")
	for _, code := range sortedLocales(shipped) {
		if !claim.codes[code] {
			t.Errorf("%s:%d states the shipped locales and leaves out %q, which %s/%s.ts ships.\n"+
				"Add %q to that list: a contributor reading it lands a new string in fewer catalogs "+
				"than the build requires, and finds out on CI.", page, claim.line, code, root, code, code)
		}
	}
	for _, code := range sortedLocales(claim.codes) {
		if !shipped[code] {
			t.Errorf("%s:%d claims locale %q, and %s/%s.ts does not exist.\n"+
				"Remove %q from that list, or add the catalog the page promises.",
				page, claim.line, code, root, code, code)
		}
	}
}

func sortedLocales(codes map[string]bool) []string {
	ordered := make([]string, 0, len(codes))
	for code := range codes {
		ordered = append(ordered, code)
	}
	sort.Strings(ordered)
	return ordered
}

// localeClaimsIn splits a page into the units a reader takes as one statement —
// a bullet with its wrapped continuation lines, a table row, a paragraph — and
// returns those that name a locale.
//
// The split is what makes the comparison honest in both directions. The copy
// table row spells three catalogs in three separate backtick spans, so a matcher
// reading spans one at a time sees three statements of one locale each and calls
// every one of them incomplete; a matcher reading single lines cuts the bullet's
// sentence at the right margin. Fenced blocks are skipped: the directory diagram
// draws paths, it does not state a set.
func localeClaimsIn(t *testing.T, page string) []localeClaim {
	t.Helper()
	raw, err := os.ReadFile(page)
	if err != nil {
		t.Fatalf("reading %s: %v", page, err)
	}
	var claims []localeClaim
	var region []string
	first := 0
	flush := func() {
		if len(region) == 0 {
			return
		}
		text := strings.Join(region, "\n")
		region = nil
		if codes := localesClaimedIn(text); len(codes) > 0 {
			claims = append(claims, localeClaim{line: first, region: text, codes: codes})
		}
	}
	inFence := false
	for i, line := range strings.Split(string(raw), "\n") {
		if fence.MatchString(line) {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		blank := strings.TrimSpace(line) == ""
		if blank || opensLocaleRegion(line) {
			flush()
		}
		if blank {
			continue
		}
		if len(region) == 0 {
			first = i + 1
		}
		region = append(region, line)
	}
	flush()
	return claims
}

// opensLocaleRegion reports whether a line begins a statement of its own rather
// than continuing the one above it: a bullet and a table row do, a wrapped line
// does not.
func opensLocaleRegion(line string) bool {
	return strings.HasPrefix(strings.TrimLeft(line, " "), "- ") || strings.HasPrefix(line, "|")
}

// localesClaimedIn returns the codes one statement names, in the two spellings
// the pages use: a catalog filename, and a parenthesised list of bare codes.
func localesClaimedIn(region string) map[string]bool {
	claimed := map[string]bool{}
	for _, span := range codeSpan.FindAllString(region, -1) {
		if named := localeCatalogPath.FindStringSubmatch(strings.Trim(span, "`")); named != nil {
			claimed[named[1]] = true
		}
	}
	for _, group := range localeListGroup.FindAllStringSubmatch(region, -1) {
		for _, code := range localeListMembers(group[1]) {
			claimed[code] = true
		}
	}
	return claimed
}

// localeListMembers reads a parenthesised run, or nothing when the run is not a
// list of locales.
//
// Every member must be a bare two-letter code and there must be at least two of
// them. Both halves carry weight: a mixed run is a pair of filenames or an
// ordinary clause, and a lone `(no)` is a word in a sentence rather than
// Norwegian. An installation down to ONE shipped locale would go unrecognised
// here — the gate then reports the page as stating nothing at all, which fails
// rather than passing quietly.
func localeListMembers(group string) []string {
	members := strings.Split(group, ",")
	if len(members) < 2 {
		return nil
	}
	codes := make([]string, 0, len(members))
	for _, member := range members {
		code := strings.Trim(strings.TrimSpace(member), "`")
		if !localeCode.MatchString(code) {
			return nil
		}
		codes = append(codes, code)
	}
	return codes
}

// The codes below are written out because this is the matcher's own spec: it
// asserts what the shapes mean, not what the tree ships.
func TestTheLocaleMatcherReadsListsAndNotProse(t *testing.T) {
	t.Parallel()
	for _, probe := range []struct {
		shape, statement string
		want             []string
	}{
		{"a bare parenthesised list", "every locale the installation ships (en, de, vi) or generation refuses",
			[]string{"de", "en", "vi"}},
		{"a backticked parenthesised list", "three catalogs (`en`, `de`, `vi`), `en` the default",
			[]string{"de", "en", "vi"}},
		{"catalog filenames across one row", "| copy | `src/i18n/en.ts`, `de.ts` **and** `vi.ts` |",
			[]string{"de", "en", "vi"}},
		{"a file whose basename is not a code", "`App.tsx`, `i18n.test.ts` and `format/plural.ts`", nil},
		{"a region-tagged language", "unconfigured English is `en-GB`, not `en-US`", nil},
		{"a parenthesised pair of filenames", "advisories (`economybanner.tsx`, `embedreindexbanner.tsx`)", nil},
		{"a two-letter word in parentheses", "the answer is (no) for now", nil},
	} {
		t.Run(probe.shape, func(t *testing.T) {
			t.Parallel()
			if got := sortedLocales(localesClaimedIn(probe.statement)); !slices.Equal(got, probe.want) {
				t.Errorf("%s: read %v as the locales named, want %v", probe.shape, got, probe.want)
			}
		})
	}
}
