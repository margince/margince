// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// The outgoing allowance counts what its name says: a call that sends something
// out of the workspace. AI work that stays inside spends a change instead, even
// on the enrich cap, and the agent guide's Egress column says what the served
// list says.

import (
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func servedSpecs() map[string]mcp.ToolSpec {
	specs := map[string]mcp.ToolSpec{}
	for _, spec := range compose.NewRegistry(nil, compose.SendPath{}).Specs() {
		specs[spec.Name] = spec
	}
	return specs
}

func TestOnlyACallThatLeavesSpendsTheOutgoingAllowance(t *testing.T) {
	t.Parallel()
	specs := servedSpecs()
	for verb, want := range map[string]agentvolume.Counter{
		"regenerate_company_brief": agentvolume.Writes,
		"ask_about_company":        agentvolume.Writes,
		"save_my_email_signature":  agentvolume.Writes,
		"run_contact_research":     agentvolume.Egress,
		"send_privacy_notice":      agentvolume.Egress,
	} {
		spec, served := specs[verb]
		if !served {
			t.Fatalf("%s is not on the served surface, so this case proves nothing", verb)
		}
		if got := agentvolume.CounterFor(spec); got != want {
			t.Errorf("%s spends the %s allowance, want %s", verb, got, want)
		}
	}
}

// guideToolRow reads a row of the guide's tool table: the tool, then its tier,
// scope and Egress cells.
var guideToolRow = regexp.MustCompile("^\\| `([a-z_]+)` \\| [^|]+ \\| [^|]+ \\| (yes|no) \\|")

func TestTheGuidesEgressColumnIsTheServedOne(t *testing.T) {
	t.Parallel()
	page, err := gatekit.ReadFile(catalogPage)
	if err != nil {
		t.Fatalf("reading %s: %v", catalogPage, err)
	}
	specs, compared := servedSpecs(), 0
	for line := range strings.SplitSeq(string(page), "\n") {
		row := guideToolRow.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		spec, listed := specs[row[1]]
		if !listed {
			continue
		}
		compared++
		if documented := row[2] == "yes"; documented != spec.Egress {
			t.Errorf("%s says %s leaves Margince: %s; the served list says %t", catalogPage, row[1], row[2], spec.Egress)
		}
	}
	// The guide lists every published tool; a short count means the table
	// moved and this compared too little to mean anything.
	if compared < 40 {
		t.Fatalf("only %d rows of %s were compared against the served list", compared, catalogPage)
	}
}

// A spec's Egress flag tells an operator the act leaves the workspace; its scope
// is what the passport pays for it. A send or enrich act may stay inside only
// as a route verb whose contract operation declares `egress: false`.
func TestEverySpecsEgressAgreesWithItsScope(t *testing.T) {
	t.Parallel()
	staysInside := map[string]bool{}
	for _, verb := range compose.RouteVerbs() {
		staysInside[verb.Verb] = verb.Scope.Egresses() && !verb.Egress
	}
	inside := 0
	for name, spec := range servedSpecs() {
		if staysInside[name] {
			inside++
		}
		if want := spec.RequiredScope.Egresses() && !staysInside[name]; spec.Egress != want {
			t.Errorf("%s declares Egress=%v but spends the %q cap, and the contract says egress is %v — "+
				"an operator reading the tool surface would be told the wrong thing about where this act goes",
				name, spec.Egress, spec.RequiredScope, want)
		}
	}
	if inside == 0 {
		t.Fatal("the contract declares no operation that stays inside, so half of this sweep checked nothing")
	}
}
