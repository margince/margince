// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Which function a LinkedIn connection works in, read from the job title the
// export carries.
//
// The reach card says where a colleague's connections work, never who they
// are. On a small account one title names one contact. So a title is folded
// into one function word and counted, and never shown.

import (
	"slices"
	"sort"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// reachFunctionFloor is the fewest connections a function is shown with. One
// would name the contact behind it wherever only one connection holds that title.
const reachFunctionFloor = 2

// reachFunctionVocabulary is the words that name each function, in English and
// German, the two languages LinkedIn's export arrives in.
type reachFunctionVocabulary struct {
	function crmcontracts.LinkedInReachFunction
	// words match a whole word of the title, for the short ones ("it", "hr",
	// "cfo") that would match inside unrelated words.
	words []string
	// stems match inside a word, for German compounds: "Vertriebsleiter" is a
	// word of its own and still names sales.
	stems []string
	// phrases match a run of whole words.
	phrases []string
}

// reachFunctions is checked in this order, and the first match wins. Every
// department comes before management, so "Head of IT" counts as IT, which is
// what a rep picking a colleague to ask needs to know.
var reachFunctions = []reachFunctionVocabulary{
	{
		function: crmcontracts.LinkedInReachFunctionIt,
		words:    []string{"it", "ict", "edv", "cio", "cto", "ciso", "devops", "sysadmin"},
		stems:    []string{"software", "developer", "entwickler", "informatik", "informationstechn"},
		phrases:  []string{"information technology", "information security", "system administrator"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionFinance,
		words:    []string{"cfo", "controller", "controlling", "accountant", "accounting", "treasurer", "treasury"},
		stems:    []string{"financ", "finanz", "buchhalt"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionSales,
		words:    []string{"sales", "cro", "sdr", "bdr"},
		stems:    []string{"vertrieb", "verkauf", "verkäufer"},
		phrases:  []string{"account executive", "account manager", "key account", "business development"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionMarketing,
		words:    []string{"cmo", "brand"},
		stems:    []string{"marketing", "kommunikation"},
		phrases:  []string{"communications manager", "head of communications"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionHr,
		words:    []string{"hr", "chro", "recruiter", "recruiting", "recruitment"},
		stems:    []string{"personalleit", "personalreferent", "personalabteilung", "personalwesen", "personalentwickl", "personalmanag"},
		phrases:  []string{"human resources", "talent acquisition", "head of people", "people culture", "people and culture"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionProcurement,
		words:    []string{"procurement", "purchasing", "purchaser", "buyer", "sourcing"},
		stems:    []string{"einkauf", "einkäufer", "beschaffung"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionOperations,
		words:    []string{"operations", "coo", "logistics", "production"},
		stems:    []string{"logistik", "produktion", "betriebsleit", "werkleit"}, //nolint:misspell // German words
		phrases:  []string{"supply chain", "plant manager", "chief operating officer"},
	},
	{
		function: crmcontracts.LinkedInReachFunctionManagement,
		words:    []string{"ceo", "founder", "cofounder", "owner", "inhaber", "president", "präsident", "chairman"},
		stems:    []string{"geschäftsführ", "gründer", "vorstand"},
		phrases:  []string{"managing director", "general manager", "managing partner", "chief executive"},
	},
}

// reachFunctionOf names the function a job title works in, and false when no
// function's words appear in it. Seniority alone ("Director", "Head of") names
// no function.
func reachFunctionOf(title string) (crmcontracts.LinkedInReachFunction, bool) {
	words := strings.FieldsFunc(strings.ToLower(title), func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	if len(words) == 0 {
		return "", false
	}
	joined := " " + strings.Join(words, " ") + " "
	for _, vocabulary := range reachFunctions {
		if vocabulary.matches(words, joined) {
			return vocabulary.function, true
		}
	}
	return "", false
}

func (v reachFunctionVocabulary) matches(words []string, joined string) bool {
	for _, word := range words {
		if slices.Contains(v.words, word) {
			return true
		}
		for _, stem := range v.stems {
			if strings.Contains(word, stem) {
				return true
			}
		}
	}
	for _, phrase := range v.phrases {
		if strings.Contains(joined, " "+phrase+" ") {
			return true
		}
	}
	return false
}

// reachFunctionCounts keeps the functions at least reachFunctionFloor
// connections work in. Most first and then by name, so two reads of an
// unchanged network agree.
func reachFunctionCounts(tally map[crmcontracts.LinkedInReachFunction]int) []crmcontracts.LinkedInReachFunctionCount {
	out := make([]crmcontracts.LinkedInReachFunctionCount, 0, len(tally))
	for function, n := range tally {
		if n >= reachFunctionFloor {
			out = append(out, crmcontracts.LinkedInReachFunctionCount{Function: function, Connections: n})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Connections != out[j].Connections {
			return out[i].Connections > out[j].Connections
		}
		return out[i].Function < out[j].Function
	})
	return out
}

// ColleagueReach is one colleague's reach into one account: how many of their
// connections work there, and the functions enough of them work in to show.
type ColleagueReach struct {
	Connections int
	Functions   []crmcontracts.LinkedInReachFunctionCount
}

// reachTally counts a colleague's connections as the rows arrive.
type reachTally struct {
	connections int
	functions   map[crmcontracts.LinkedInReachFunction]int
}

func (t *reachTally) add(position string) {
	t.connections++
	if function, ok := reachFunctionOf(position); ok {
		t.functions[function]++
	}
}

// tallyReach reads CompanyLinkedInReach's rows, one per connection. The titles
// are folded into functions here and go no further.
func tallyReach(rows pgx.Rows) (map[ids.UUID]ColleagueReach, error) {
	tallies := map[ids.UUID]*reachTally{}
	for rows.Next() {
		var user ids.UUID
		var position string
		if err := rows.Scan(&user, &position); err != nil {
			return nil, err
		}
		if tallies[user] == nil {
			tallies[user] = &reachTally{functions: map[crmcontracts.LinkedInReachFunction]int{}}
		}
		tallies[user].add(position)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]ColleagueReach, len(tallies))
	for user, tally := range tallies {
		out[user] = ColleagueReach{
			Connections: tally.connections,
			Functions:   reachFunctionCounts(tally.functions),
		}
	}
	return out, nil
}
