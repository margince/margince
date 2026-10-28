// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// TestEveryExactSeatMatchFoldsTheDomain holds every SQL comparison of a seat's
// login to storekit.SeatLoginIsSQL. A login stored in Unicode and mail in
// punycode are one mailbox, and the raw comparison reads them as two.
//
// Each lowercased email operand is judged alone, never one per line. Its
// table comes from the file's `FROM`, `JOIN` and `UPDATE` bindings. An alias
// the file never binds is a fragment spliced into a statement elsewhere. It
// counts as a seat match, because reading short is the failure to avoid.
func TestEveryExactSeatMatchFoldsTheDomain(t *testing.T) {
	t.Parallel()
	sites := rawSeatMatches(t)
	var unregistered []string
	for _, site := range sites {
		if _, known := rawSeatMatchDebt[site.file]; !known {
			unregistered = append(unregistered, site.String())
		}
	}
	if len(unregistered) > 0 {
		sort.Strings(unregistered)
		t.Errorf("%d comparison(s) read a seat's login in its raw spelling:\n\t%s\n"+
			"Compare through storekit.SeatLoginIsSQL, which folds the login's domain the way "+
			"owndomain.Fold folds the address it is matched to.", len(unregistered), strings.Join(unregistered, "\n\t"))
	}
	for file := range rawSeatMatchDebt {
		if !hasSiteIn(sites, file) {
			t.Errorf("rawSeatMatchDebt lists %s, which has no raw seat match left: delete the entry", file)
		}
	}
}

// emailOperand is one side of a comparison that reads an email column
// lowercased, with the alias it names, if any.
var emailOperand = regexp.MustCompile(`(?i)lower\(\s*(?:([a-z_]\w*)\.)?email\s*\)`)

var (
	comparedAfter  = regexp.MustCompile(`(?i)^\s*(=|<>|!=|\bIN\b|\bNOT\s+IN\b)`)
	comparedBefore = regexp.MustCompile(`(=|<>|!=)\s*$`)
)

// rawSeatMatchDebt lists the files that still compare a seat's login by its
// raw spelling. It is closed: it only shrinks, and an entry with no raw match
// left fails, so it states the tree as it is.
//
// gatekit:fixture the file, and why its match cannot read the folded login yet.
var rawSeatMatchDebt = map[string]string{
	"internal/modules/activities/waitingsql.go":      "compares a seat's login to a contact_email row, which stores no folded domain",
	"internal/modules/contacts/membernumbers.go":     "compares a seat's login to a contact_email row, which stores no folded domain",
	"internal/modules/identity/federatedidentity.go": "a sign-in compares the typed address to the login as registered",
	"internal/modules/identity/former.go":            "a sign-in compares the typed address to the login as registered",
	"internal/modules/identity/lockout.go":           "a sign-in compares the typed address to the login as registered",
	"internal/modules/identity/maillanguage.go":      "a sign-in compares the typed address to the login as registered",
	"internal/modules/identity/membereligibility.go": "a sign-in compares the typed address to the login as registered",
}

type seatMatchSite struct {
	file, alias string
	line        int
}

func (s seatMatchSite) String() string {
	return s.file + ":" + strconv.Itoa(s.line) + " lower(" + s.alias + "email)"
}

func hasSiteIn(sites []seatMatchSite, file string) bool {
	for _, site := range sites {
		if site.file == file {
			return true
		}
	}
	return false
}

func rawSeatMatches(t *testing.T) []seatMatchSite {
	t.Helper()
	var sites []seatMatchSite
	operands := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		source := string(body)
		aliases := boundTables(source)
		for _, at := range emailOperand.FindAllStringSubmatchIndex(source, -1) {
			if !comparedAfter.MatchString(source[at[1]:]) && !comparedBefore.MatchString(source[:at[0]]) {
				continue
			}
			operands++
			alias := ""
			if at[2] >= 0 {
				alias = strings.ToLower(source[at[2]:at[3]])
			}
			if !readsSeatLogin(source[:at[0]], alias, aliases) {
				continue
			}
			label := ""
			if alias != "" {
				label = alias + "."
			}
			sites = append(sites, seatMatchSite{file: path, alias: label, line: strings.Count(source[:at[0]], "\n") + 1})
		}
	}
	// The tree held 50 compared email operands when this landed. Far fewer
	// means the pattern stopped reading them, and every one would pass.
	if operands < 40 {
		t.Fatalf("this census judged %d compared email operand(s) and expects at least 40: "+
			"it has stopped recognising them", operands)
	}
	return sites
}

// boundTables maps each alias a file binds to the tables it names.
func boundTables(source string) map[string]map[string]bool {
	aliases := map[string]map[string]bool{}
	for _, m := range tableAlias.FindAllStringSubmatch(source, -1) {
		alias := strings.ToLower(m[2])
		if alias == "" || sqlKeywords[alias] {
			continue
		}
		if aliases[alias] == nil {
			aliases[alias] = map[string]bool{}
		}
		aliases[alias][strings.ToLower(m[1])] = true
	}
	return aliases
}

// readsSeatLogin reports whether the operand reads app_user's email. A bare
// column belongs to the nearest table read before it.
func readsSeatLogin(before, alias string, aliases map[string]map[string]bool) bool {
	if alias != "" {
		tables, bound := aliases[alias]
		return !bound || tables["app_user"]
	}
	bindings := tableAlias.FindAllStringSubmatch(before, -1)
	return len(bindings) == 0 || strings.EqualFold(bindings[len(bindings)-1][1], "app_user")
}
