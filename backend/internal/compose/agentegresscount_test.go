// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The outgoing allowance counts what its name says: a call that sends something
// out of the workspace. AI work that stays inside spends a change instead, even
// on the enrich cap, and the agent guide's Egress column says what the served
// list says.

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/agentvolume"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

func TestOnlyACallThatLeavesSpendsTheOutgoingAllowance(t *testing.T) {
	specs := map[string]mcp.ToolSpec{}
	for _, spec := range servedSurface(t).Specs() {
		specs[spec.Name] = spec
	}
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
	page, err := os.ReadFile(filepath.Join("..", "..", "..", "docs", "reference", "agent-tools.md"))
	if err != nil {
		t.Fatalf("reading the agent guide: %v", err)
	}
	served := map[string]bool{}
	for _, spec := range servedSurface(t).Specs() {
		served[spec.Name] = spec.Egress
	}
	compared := 0
	for _, line := range strings.Split(string(page), "\n") {
		row := guideToolRow.FindStringSubmatch(line)
		if row == nil {
			continue
		}
		egress, listed := served[row[1]]
		if !listed {
			continue
		}
		compared++
		if documented := row[2] == "yes"; documented != egress {
			t.Errorf("the guide says %s leaves Margince: %s; the served list says %t", row[1], row[2], egress)
		}
	}
	// The guide lists every published tool; a short count means the table
	// moved and this compared too little to mean anything.
	if compared < 40 {
		t.Fatalf("only %d guide rows were compared against the served list", compared)
	}
}
