// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
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
var emailOperand = regexp.MustCompile(`(?i)lower\s*\(\s*(?:([a-z_]\w*)\.)?email\s*\)`)

// loweredValueOperand is the other spelling of the same comparison: the raw
// column compared to a lowercased value, as in `email = lower($1)`. It reads
// the login unfolded just as the first does.
var loweredValueOperand = regexp.MustCompile(`(?i)(?:\b([a-z_]\w*)\.)?\bemail\s*=\s*lower\s*\(`)

// commutedValueOperand is that comparison written the other way round, as in
// `lower($1) = u.email`. The column it names is the second group's alias.
var commutedValueOperand = regexp.MustCompile(`(?i)lower\s*\([^()]*\)\s*=\s*((?:\b([a-z_]\w*)\.)?\bemail)\b`)

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
	"internal/modules/activities/waitingsql.go":  "compares a seat's login to a contact_email row, which stores no folded domain",
	"internal/modules/contacts/membernumbers.go": "compares a seat's login to a contact_email row, which stores no folded domain",
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
		body, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		found, judged := seatMatchesIn(path, string(body))
		sites = append(sites, found...)
		operands += judged
	}
	// A comparison that moves to storekit.SeatLoginIsSQL leaves this census. So
	// the floor falls as the debt is paid, and it is not a tree-wide count.
	// It catches the pattern recognising nothing, which reads as zero.
	if operands < 33 {
		t.Fatalf("this census judged %d compared email operand(s) and expects at least 33: "+
			"it has stopped recognising them", operands)
	}
	return sites
}

// seatMatchesIn reads one file's raw seat-login comparisons, in each of the
// spellings above, and answers how many compared email operands it judged.
func seatMatchesIn(path, source string) ([]seatMatchSite, int) {
	var sites []seatMatchSite
	operands := 0
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
	for _, at := range loweredValueOperand.FindAllStringSubmatchIndex(source, -1) {
		// An alias spliced in from Go (`alias + ".email = ..."`) is a helper's
		// fragment for some other table, which the lower(alias.email) reading
		// above cannot see either.
		if at[0] >= 2 && source[at[0]-2:at[0]] == "\"." {
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
	for _, at := range commutedValueOperand.FindAllStringSubmatchIndex(source, -1) {
		// A lowercased column on the left is the first spelling's operand, judged
		// above. This one is a lowercased value compared to a raw column.
		if strings.Contains(strings.ToLower(source[at[0]:at[2]]), "email") {
			continue
		}
		if at[2] >= 2 && source[at[2]-2:at[2]] == "\"." {
			continue
		}
		operands++
		alias := ""
		if at[4] >= 0 {
			alias = strings.ToLower(source[at[4]:at[5]])
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
	return sites, operands
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

// The census reads each spelling of a raw seat-login comparison, and none of
// the folded one. Planted, because a spelling it misses reports a clean tree.
func TestTheSeatMatchCensusReadsEverySpelling(t *testing.T) {
	t.Parallel()
	for _, statement := range []string{
		"SELECT 1 FROM app_user u WHERE lower(u.email) = lower($1)",
		"SELECT 1 FROM app_user u WHERE lower (u.email) = $1",
		"SELECT 1 FROM app_user u WHERE u.email = lower($1)",
		"SELECT 1 FROM app_user u WHERE u.email = lower ($1)",
		"SELECT 1 FROM app_user u WHERE lower($1) = u.email",
	} {
		if sites, _ := seatMatchesIn("planted.go", "`"+statement+"`"); len(sites) != 1 {
			t.Errorf("the census reads %d raw comparison(s) in %q, want 1", len(sites), statement)
		}
	}
	for _, statement := range []string{
		"SELECT 1 FROM app_user u WHERE u.email_domain_folded = $2",
		"SELECT 1 FROM contact_email ce WHERE ce.email = lower($1)",
		"SELECT c.email FROM unnest($1::text[]) c(email) JOIN contact_email ce ON true WHERE lower(ce.email) = c.email",
	} {
		if sites, _ := seatMatchesIn("planted.go", "`"+statement+"`"); len(sites) != 0 {
			t.Errorf("the census reads %q as a raw seat-login comparison", statement)
		}
	}
}
