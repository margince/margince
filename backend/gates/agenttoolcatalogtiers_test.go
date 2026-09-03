// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The tool catalog's Tier column says what the contract says.
//
// docs/reference/agent-tools.md is what somebody deciding whether to hand an
// agent a passport reads — not agentpolicy_gen.go. It states, per tool, whether
// the verb RUNS or waits for a human, and it is hand-kept. A wrong tier there
// tells somebody a consequential verb waits for them when it does not, which is
// worse than telling them nothing.
//
// So both sides are DERIVED and compared. agentPolicies is generated from
// crm.yaml, which means an annotation change binds this page by regeneration
// rather than by anyone remembering to edit it.
//
// THE MIXED CASE IS THE POINT, not an exception to skip. create_record and
// update_record resolve per record type, so no single mark can be right for
// them: the doc writes 🟢 / 🟡 and the contract carries two tiers under one tool
// name. A gate that skipped them would be blind to the half of the table most
// likely to mislead — so a mixed tool must be written mixed, and a tool whose
// policies agree may not claim to be.

import (
	"os"
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// catalogPage is the page this gate holds. Relative to the backend module, like
// every other root a gate here walks.
const catalogPage = "../docs/reference/agent-tools.md"

// catalogRow reads one table row: the tool in backticks, then the tier cell.
// The remaining columns are not this gate's subject and are left unread.
var catalogRow = regexp.MustCompile("^\\|\\s*`([a-z0-9_]+)`\\s*\\|\\s*([^|]+?)\\s*\\|")

// catalogHeader opens the table this gate reads. The page holds a SECOND table
// of the same row shape — the passport scopes, keyed by scope name — and a scan
// that took every matching row read `enrich` as tier "1", which is that table's
// route count. Rows are therefore read only between this header and the blank
// line that ends its table.
const catalogHeader = "| Tool | Tier | Scope | Egress | In overlay mode |"

// The marks the page uses, and what each means in the contract's own
// vocabulary. Spelled here rather than inferred, because the mapping IS the
// claim being checked: a gate that derived it from the page would agree with
// whatever the page said.
const (
	marksRuns      = "🟢"
	marksWaits     = "🟡"
	marksDynamic   = "dynamic"
	marksBothTiers = "🟢 / 🟡"
)

// tierMark renders one contract tier as the page writes it.
func tierMark(tier string) string {
	switch tier {
	case "auto_execute":
		return marksRuns
	case "confirmation_required":
		return marksWaits
	case "dynamic":
		return marksDynamic
	default:
		return ""
	}
}

func TestTheToolCatalogsTiersAreTheContractsTiers(t *testing.T) {
	t.Parallel()
	// An entry that matched no subject reads as ratification of a verb that has
	// been listed since — which is the direction this baseline is supposed to
	// move in, and it must be noticed rather than left standing.
	defer absentFromTheCatalog.AssertAllMatched(t)
	page, err := os.ReadFile(catalogPage)
	if err != nil {
		t.Fatalf("reading %s: %v", catalogPage, err)
	}

	documented := map[string]string{}
	inTable := false
	for _, line := range strings.Split(string(page), "\n") {
		if strings.TrimSpace(line) == catalogHeader {
			inTable = true
			continue
		}
		if !inTable {
			continue
		}
		if strings.TrimSpace(line) == "" {
			inTable = false
			continue
		}
		if match := catalogRow.FindStringSubmatch(line); match != nil {
			documented[match[1]] = strings.TrimSpace(match[2])
		}
	}
	if len(documented) == 0 {
		t.Fatalf("no tool rows parsed out of %s — this gate is reading a table shape that is gone, "+
			"and an empty census passes exactly like a correct page", catalogPage)
	}

	declared := compose.AgentToolTiers()
	if len(declared) == 0 {
		t.Fatal("the contract declares no tool tiers, so this gate is comparing the page against nothing")
	}

	compared := 0
	for _, tool := range catalogToolNames(documented) {
		tiers, governed := declared[tool]
		if !governed {
			// A row the contract declares no tier for. Most of the catalog is
			// this: a native read tool is registered by the MCP registry and
			// has no crm.yaml operation, so agentPolicies has nothing to say
			// about it. Not a finding — this gate holds the tier column
			// against the contract, and where there is no contract tier there
			// is nothing to hold it against.
			continue
		}
		compared++
		if want := markFor(tiers); documented[tool] != want {
			t.Errorf("%s says %s is %q; the contract says %q. This page is what somebody reads before "+
				"granting a passport, so a wrong tier here is worse than an absent one",
				catalogPage, tool, documented[tool], want)
		}
	}
	// A comparison of nothing passes exactly like a correct page.
	//
	// The floor is CLOSE to the real count — twenty-one rows carry a contract
	// tier today — so it is not a robustness margin and must not be read as
	// one. It trips when the scan breaks or the governed corpus shrinks, and
	// either is a thing to look at: the first is this gate reading a table
	// shape that has moved, the second is verbs leaving the contract's
	// governance, which is not a change to make quietly.
	if compared < 20 {
		t.Fatalf("only %d row(s) were compared against a contract tier, so this gate covered almost "+
			"nothing — the table shape or the policy table has moved", compared)
	}

	var arrived []string
	for _, tool := range catalogToolNames(declared) {
		if _, listed := documented[tool]; listed {
			continue
		}
		if absentFromTheCatalog.Waived(t, tool) {
			continue
		}
		arrived = append(arrived, tool)
	}
	if len(arrived) > 0 {
		t.Errorf("the contract declares a tier for %v and %s does not list them. The catalog is the "+
			"inventory a reader consults before granting a passport, so a governed verb missing from it "+
			"is a verb whose autonomy nobody can look up", arrived, catalogPage)
	}
}

// absentFromTheCatalog is EMPTY, and that is the state to keep it in.
//
// It carried nineteen entries when this gate landed — governed verbs the
// hand-kept page had never listed — ratcheted rather than swept because filling
// a row in means writing the verb's overlay-mode behaviour, which is read out
// of the code per verb rather than guessed at nineteen times. They have since
// been read and written, and the backlog is gone.
//
// So a new entry here is not a smaller version of that backlog: it is one verb
// whose row somebody did not write, on the page a reader consults before
// granting a passport. Write the row instead. If a verb genuinely cannot have
// one, the reason belongs here in its own words, not a shared constant.
var absentFromTheCatalog = gatekit.Waive(map[string]string{})

// markFor is how the page must write a tool whose policies carry these tiers.
//
// One tier is written as itself. Two are written as the mixed mark, whatever
// the two are: the page's own legend says the mixed mark means "the tier
// depends on the record type", which is true of any tool that resolves more
// than one.
func markFor(tiers []string) string {
	marks := map[string]bool{}
	for _, tier := range tiers {
		if mark := tierMark(tier); mark != "" {
			marks[mark] = true
		}
	}
	if len(marks) == 1 {
		for mark := range marks {
			return mark
		}
	}
	if marks[marksRuns] && marks[marksWaits] {
		return marksBothTiers
	}
	// Any other combination is a shape the page has no vocabulary for, and
	// naming it here beats rendering something that looks like a mark.
	return "an unwritable combination of " + strings.Join(sortedSet(marks), " and ")
}

func sortedSet(in map[string]bool) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}

// catalogToolNames is the tool names in a table, sorted, so both walks report in
// a stable order. Generic over the value because the two tables it reads answer
// different questions with the same key.
func catalogToolNames[V any](in map[string]V) []string {
	out := make([]string, 0, len(in))
	for key := range in {
		out = append(out, key)
	}
	sort.Strings(out)
	return out
}
