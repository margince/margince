// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// The team week names coverage only against its basis.
//
// The product has no authoritative target, so no copy in any locale may say a
// team is "off target" or "not on track". Coverage is "at risk against" the
// call or the historical median it was measured by. The corpus comes from its three
// owners: the weekly package's Go labels, the TeamWeekly schema descriptions
// and the teamweekly catalog entries.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// targetWording is the wording, per catalog locale, that claims a target the
// product does not have. A locale without an entry fails copyFindings.
var targetWording = map[string]*regexp.Regexp{
	"en": regexp.MustCompile(`(?i)off[ -]target|on[ -]target|not on track|behind target`),
	"de": regexp.MustCompile(`(?i)ziel verfehlt|zielvorgabe|nicht auf kurs|hinter (?:dem|plan)`),
	"vi": regexp.MustCompile(`(?i)chệch mục tiêu|lệch mục tiêu|không đúng tiến độ|chậm tiến độ`),
}

var (
	// "Coverage at risk against" and what follows it, in English prose.
	coverageAgainst = regexp.MustCompile(`(?is)coverage\s+at\s+risk\s+against\b(.{0,24})`)
	// What may follow "against": a basis, named or interpolated.
	namedBasis = regexp.MustCompile(
		`(?i)^\s*(?:\{basis\}|%s|<basis>|(?:a|the)\s+named\s+basis|its\s+basis|the\s+manager['’]s\s+call|the\s+historical\s+median)`)
	// One teamweekly catalog entry, its value on the same or the next line.
	teamWeeklyCatalogEntry = regexp.MustCompile(`"(teamweekly\.[^"]+)":\s*"((?:[^"\\]|\\.)*)"`)
	// An English catalog value that states coverage against something.
	coverageStatement = regexp.MustCompile(`(?i)coverage .*against`)
)

func TestTheTeamWeekNamesCoverageOnlyAgainstItsBasis(t *testing.T) {
	t.Parallel()
	english := teamWeeklyGoStrings(t)
	maps.Copy(english, teamWeeklyContractText(t))
	catalogs := map[string]map[string]string{}
	for _, locale := range []string{"en", "de", "vi"} {
		catalogs[locale] = teamWeeklyCatalog(t, locale)
		// A scan that found nothing agrees with every rule.
		if len(catalogs[locale]) < 50 {
			t.Fatalf("read %d teamweekly entries from %s: the scan stopped seeing its subject",
				len(catalogs[locale]), locale)
		}
	}
	for where, text := range english {
		for _, finding := range copyFindings("en", text) {
			t.Errorf("%s: %s", where, finding)
		}
	}
	for locale, catalog := range catalogs {
		for key, text := range catalog {
			for _, finding := range copyFindings(locale, text) {
				t.Errorf("%s:%s: %s", locale, key, finding)
			}
		}
	}
	statements := coverageKeys(catalogs["en"])
	if len(statements) < 2 {
		t.Fatalf("only %d catalog keys state coverage against a basis: the copy rule has no subject", len(statements))
	}
	for locale, catalog := range catalogs {
		for _, finding := range basisPlaceholderMissing(statements, catalog) {
			t.Errorf("%s: %s", locale, finding)
		}
	}
}

// The rule must fail each wrong spelling it exists for, in every locale.
func TestTheCopyRuleFailsATargetClaimInEveryLocale(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct{ locale, text string }{
		{"en", "Coverage is off target"},
		{"de", "Deckung ist nicht auf Kurs"},
		{"vi", "Độ phủ chệch mục tiêu"},
		{"en", "Coverage at risk against"},
		{"en", "Coverage at risk against the plan"},
		{"fr", "Couverture en retard"},
	} {
		if len(copyFindings(tc.locale, tc.text)) == 0 {
			t.Errorf("%s %q passed the copy rule", tc.locale, tc.text)
		}
	}
	if got := copyFindings("en", "Coverage at risk against {basis}"); len(got) != 0 {
		t.Errorf("the right wording failed: %v", got)
	}
	keys := []string{"teamweekly.coverage.atRisk"}
	if len(basisPlaceholderMissing(keys, map[string]string{keys[0]: "Deckung gefährdet"})) == 0 {
		t.Error("a translation that drops the basis passed")
	}
	if len(basisPlaceholderMissing(keys, map[string]string{})) == 0 {
		t.Error("a translation that lacks the key passed")
	}
}

// copyFindings names what one string in one locale says wrongly.
func copyFindings(locale, text string) []string {
	forbidden, known := targetWording[locale]
	if !known {
		return []string{"no target wording is declared for locale " + locale}
	}
	var out []string
	if forbidden.MatchString(text) {
		out = append(out, fmt.Sprintf("%q claims a target the product does not have", text))
	}
	if locale == "en" {
		for _, m := range coverageAgainst.FindAllStringSubmatch(text, -1) {
			if !namedBasis.MatchString(m[1]) {
				out = append(out, fmt.Sprintf("%q says coverage is at risk against no named basis", m[0]))
			}
		}
	}
	return out
}

// coverageKeys are the English catalog keys that state coverage against
// something: their translations must interpolate the basis too.
func coverageKeys(english map[string]string) []string {
	var keys []string
	for key, text := range english {
		if coverageStatement.MatchString(text) {
			keys = append(keys, key)
		}
	}
	return keys
}

// basisPlaceholderMissing names the keys whose value lacks the basis.
func basisPlaceholderMissing(keys []string, catalog map[string]string) []string {
	var out []string
	for _, key := range keys {
		if text, found := catalog[key]; !found || !strings.Contains(text, "{basis}") {
			out = append(out, fmt.Sprintf("%s reads %q, which does not name the basis", key, text))
		}
	}
	return out
}

// teamWeeklyCatalog reads one locale's teamweekly entries.
func teamWeeklyCatalog(t *testing.T, locale string) map[string]string {
	t.Helper()
	path := filepath.Join("..", "frontend", "src", "i18n", locale+".ts")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	out := map[string]string{}
	for _, m := range teamWeeklyCatalogEntry.FindAllStringSubmatch(string(raw), -1) {
		out[m[1]] = m[2]
	}
	return out
}

// teamWeeklyGoStrings are the string literals of the weekly package's team
// files, which compose the labels and the summary.
func teamWeeklyGoStrings(t *testing.T) map[string]string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join("internal", "compose", "weekly", "team*.go"))
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	fset := token.NewFileSet()
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", file, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				return true
			}
			if text, err := strconv.Unquote(lit.Value); err == nil {
				out[fset.Position(lit.Pos()).String()] = text
			}
			return true
		})
	}
	return out
}

// teamWeeklyContractText collects the descriptions under the TeamWeekly schemas.
func teamWeeklyContractText(t *testing.T) map[string]string {
	t.Helper()
	schemas, ok := loadContractDocument(t)["components"].(map[string]any)["schemas"].(map[string]any)
	if !ok {
		t.Fatal("the contract has no components.schemas")
	}
	out := map[string]string{}
	for name, schema := range schemas {
		if strings.HasPrefix(name, "TeamWeekly") {
			collectDescriptions(schema, "crm.yaml:"+name, out)
		}
	}
	return out
}

// collectDescriptions walks one schema's untyped YAML nodes.
//
//craft:ignore naked-any the contract is read as untyped YAML nodes, as loadContractDocument returns it
func collectDescriptions(node any, where string, out map[string]string) {
	switch v := node.(type) {
	case map[string]any:
		for key, child := range v {
			if text, ok := child.(string); ok && key == "description" {
				out[where] = text
				continue
			}
			collectDescriptions(child, where+"."+key, out)
		}
	case []any:
		for i, child := range v {
			collectDescriptions(child, where+"."+strconv.Itoa(i), out)
		}
	}
}
