// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A column a CHECK holds in lower case is compared bare. Its index is on the
// plain column, which Postgres does not use for `lower(email) = ...`. It does
// not reason from the CHECK either, so the wrapped comparison reads the table.
// Dropping lower() around such a column never changes which rows match.

import (
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// lowerCaseCheck is a catalog CHECK of the form `col = lower(col)`; the code
// compares the two column names, which a Go regexp cannot.
var lowerCaseCheck = regexp.MustCompile(`^public\.([a-z_]\w*)\.\w+ CHECK \(.*\b([a-z_]\w*) = lower\(([a-z_]\w*)\)`)

// addressColumnsHeldLowerCase are the email columns the address lookups read.
// The census must find each by name: a reader that lost them could still count
// the many `key = lower(key)` CHECKs and look whole.
var addressColumnsHeldLowerCase = []string{"contact_email.email", "deal_room_participant.email", "lead.email"}

var (
	// lowerCaseComparedAfter and lowerCaseComparedBefore widen the seat census's
	// operators by LIKE and ILIKE, which a bare column answers the same way.
	lowerCaseComparedAfter  = regexp.MustCompile(`(?i)^\s*(=|<>|!=|\bIN\b|\bNOT\s+IN\b|\b(?:NOT\s+)?I?LIKE\b)`)
	lowerCaseComparedBefore = regexp.MustCompile(`(?i)(=|<>|!=|\bI?LIKE)\s*$`)
)

// lowerCaseColumns maps each column name to the tables whose CHECK holds it in
// lower case. It reads the committed schema, so a new one enrols itself.
func lowerCaseColumns(t *testing.T) map[string]map[string]bool {
	t.Helper()
	raw, err := gatekit.ReadFile("migrations/testdata/head_catalog.txt")
	if err != nil {
		t.Fatalf("reading the head catalog: %v", err)
	}
	columns := map[string]map[string]bool{}
	for line := range strings.SplitSeq(string(raw), "\n") {
		m := lowerCaseCheck.FindStringSubmatch(line)
		if m == nil || m[2] != m[3] {
			continue
		}
		if columns[m[2]] == nil {
			columns[m[2]] = map[string]bool{}
		}
		columns[m[2]][m[1]] = true
	}
	for _, want := range addressColumnsHeldLowerCase {
		table, column, _ := strings.Cut(want, ".")
		if !columns[column][table] {
			t.Fatalf("the catalog reader found no lower-case CHECK on %s: it stopped recognising them", want)
		}
	}
	return columns
}

// unboundLowerCaseNames holds the files whose bare column shares a name with a
// lower-case column but is spliced into a statement on another table. Keyed by
// file and operand, so a second wrapped operand in one file is still read.
var unboundLowerCaseNames = gatekit.Waive(map[string]string{
	"internal/modules/projects/keymatch.go lower(key)": "compares project.key, which has no lower-case CHECK, in a WHERE fragment the census cannot bind to its table. Cost: a second lower(key) comparison in this file goes unread",
})

func TestNoLowerCaseColumnIsComparedThroughLower(t *testing.T) {
	t.Parallel()
	defer unboundLowerCaseNames.AssertAllMatched(t)
	columns := lowerCaseColumns(t)
	var wrapped []string
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		body, err := gatekit.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		for _, site := range wrappedLowerCaseCompares(path, string(body), columns) {
			if unboundLowerCaseNames.Waived(t, site.path+" "+site.operand) {
				continue
			}
			wrapped = append(wrapped, site.String())
		}
	}
	if len(wrapped) > 0 {
		sort.Strings(wrapped)
		t.Errorf("%d comparison(s) wrap a column its CHECK already holds in lower case:\n\t%s\n"+
			"Compare the bare column (`pe.email = lower($1)`, `pe.email = lower(ap.address)`): "+
			"the plain-column index then answers it, and the same rows match.",
			len(wrapped), strings.Join(wrapped, "\n\t"))
	}
}

// lowerCaseSite is one comparison that wraps a lower-case column.
type lowerCaseSite struct {
	path, operand string
	line          int
}

func (s lowerCaseSite) String() string {
	return s.path + ":" + strconv.Itoa(s.line) + " " + s.operand
}

// wrappedLowerCaseCompares reads one file's comparisons that wrap a lower-case
// column in lower(). An alias the file never binds counts, as in the seat census.
func wrappedLowerCaseCompares(path, source string, columns map[string]map[string]bool) []lowerCaseSite {
	names := make([]string, 0, len(columns))
	for name := range columns {
		names = append(names, regexp.QuoteMeta(name))
	}
	sort.Strings(names)
	// A schema qualifier (`public.contact_email.email`) is read past; the alias
	// is the name right before the column.
	operand := regexp.MustCompile(`(?i)lower\s*\(\s*(?:(?:[a-z_]\w*\.)?([a-z_]\w*)\.)?(` + strings.Join(names, "|") + `)\s*\)`)
	aliases := boundTables(source)
	var sites []lowerCaseSite
	for _, at := range operand.FindAllStringSubmatchIndex(source, -1) {
		if !lowerCaseComparedAfter.MatchString(source[at[1]:]) && !lowerCaseComparedBefore.MatchString(source[:at[0]]) {
			continue
		}
		alias := ""
		if at[2] >= 0 {
			alias = strings.ToLower(source[at[2]:at[3]])
		}
		tables := columns[strings.ToLower(source[at[4]:at[5]])]
		if !tables[alias] && !operandReads(source[:at[0]], alias, aliases, tables) {
			continue
		}
		sites = append(sites, lowerCaseSite{path: path, line: strings.Count(source[:at[0]], "\n") + 1, operand: source[at[0]:at[1]]})
	}
	return sites
}

// The census reads each wrapped spelling on a lower-case column, and none on
// another table or outside a comparison. Planted, because a spelling it misses
// reports a clean tree.
func TestTheLowerCaseColumnCensusReadsEverySpelling(t *testing.T) {
	t.Parallel()
	columns := lowerCaseColumns(t)
	for _, statement := range []string{
		"SELECT 1 FROM contact_email pe WHERE lower(pe.email) = lower($1)",
		"SELECT 1 FROM contact_email pe WHERE lower (pe.email) = ANY($1)",
		"SELECT 1 FROM lead WHERE lower(email) = $1",
		"SELECT 1 FROM lead l WHERE $1 = lower(l.email)",
		"SELECT 1 FROM activity_participant ap JOIN contact_email ce ON lower(ap.address) = lower(ce.email)",
		"SELECT 1 FROM deal_room_participant p WHERE lower(p.email) IN ('a')",
		"SELECT 1 FROM public.contact_email WHERE lower(public.contact_email.email) = lower($1)",
		"SELECT 1 FROM contact_email ce WHERE lower(ce.email) ILIKE $1",
		"SELECT 1 FROM lead l WHERE lower(l.email) NOT LIKE $1",
		"SELECT 1 FROM lead l WHERE $1 LIKE lower(l.email)",
	} {
		if sites := wrappedLowerCaseCompares("planted.go", "`"+statement+"`", columns); len(sites) != 1 {
			t.Errorf("the census reads %d wrapped comparison(s) in %q, want 1", len(sites), statement)
		}
	}
	for _, statement := range []string{
		"SELECT 1 FROM contact_email pe WHERE pe.email = lower($1)",
		"SELECT 1 FROM app_user u WHERE lower(u.email) = lower($1)",
		"SELECT 1 FROM contact_email ce JOIN app_user u ON ce.email = lower(u.email)",
		"SELECT lower(pe.email) FROM contact_email pe",
		"SELECT 1 FROM lead l WHERE f_fold_apostrophes(lower(l.email)) % $1",
	} {
		if sites := wrappedLowerCaseCompares("planted.go", "`"+statement+"`", columns); len(sites) != 0 {
			t.Errorf("the census reads %q as a wrapped comparison: %v", statement, sites)
		}
	}
}
