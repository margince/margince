// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind budget H2

package gates

// How much of a user's tool budget this server spends.
//
// A client's tool ceiling is spent across EVERY configured MCP server, not per
// server. Antigravity caps at 100 tools in total, so what this server publishes
// is subtracted from what a user has left for everything else they connect.
//
// Nothing counted it, and this is what that costs:
//
//	33  when the budget question was first raised
//	35  at the close of the issue that raised it
//	37  when #940 was filed
//	59  at #940's ruling, 2026-08-31
//	76  today
//
// Every one of those additions was locally reasonable. The aggregate was
// nobody's job — which is the whole diagnosis, and the reason the ruling put
// the gate BEFORE the reconciliation rather than after it.
//
// WHY A COUNT AND NOT TOKENS. The listing token budget is a different bound on
// a different surface, currently green with headroom, and it explicitly
// declined its own levers. Nothing about that headroom licenses adding tools:
// this ceiling is held by the CLIENT, counts whole tools, and the two move
// independently. Reading one as the other is the mistake this paragraph exists
// to prevent.
//
// WHY THIS GATE CANNOT LOWER THE NUMBER. Merging or renaming verbs runs
// through x-mcp-tool and the generated policy against a vocabulary ratified
// outside this tree, so a local rename is a contract break. That reconciliation
// is the second half of #940 and is where the count actually comes down. A
// count gate touches none of it, which is why it did not have to wait.

import (
	"encoding/json"
	"os"
	"testing"
)

const mcpInfoPath = "../docs/reference/mcp-info.json"

// clientToolCeiling is the total a client allows across every server it is
// configured with. Named rather than folded into the arithmetic below, because
// the number that matters to a user is the SHARE this server takes of it.
const clientToolCeiling = 100

// publishedToolCeiling is the most tools this server may publish.
//
// Set at exactly today's count, with NO headroom, and that is deliberate.
//
// The ruling asked for a number that "will actually bite — a number that admits
// a small amount of planned growth and refuses the next unplanned addition —
// rather than at 59+n, which would ratify the drift". At 59 there was room to
// honour both halves of that. At 76 of 100 there is not: any headroom is spent
// from a budget already three-quarters gone, so a ceiling above the current
// count would ratify the drift AND buy the growth, which is the one combination
// the ruling rules out.
//
// So the next addition is a decision. Raising this is allowed — it is a
// constant with a comment, not a law — but it costs an argument in a pull
// request about a user's remaining budget, which is exactly the argument that
// never happened forty-three tools ago.
//
// It comes DOWN when the verb reconciliation lands, and a ceiling that did not
// follow it down would quietly re-bank the room it freed.
// Calendar sending is separately approved from recording a meeting; this costs
// one additional tool, leaving 23 of the client’s 100 slots for other servers.
const publishedToolCeiling = 77

type mcpInfoFile struct {
	Totals struct {
		Tools     int `json:"tools"`
		Resources int `json:"resources"`
	} `json:"totals"`
}

func TestThePublishedToolCountStaysUnderItsCeiling(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(mcpInfoPath)
	if err != nil {
		t.Fatalf("reading %s: %v", mcpInfoPath, err)
	}
	var info mcpInfoFile
	if err := json.Unmarshal(raw, &info); err != nil {
		t.Fatalf("parsing %s: %v", mcpInfoPath, err)
	}
	// A zero here is the under-recognising failure this directory's rules warn
	// about: the file moved or its shape changed, the count reads as nothing,
	// and a ceiling comparison passes while measuring no surface at all.
	if info.Totals.Tools == 0 {
		t.Fatalf("%s reports no tools — the published surface moved or its shape changed, "+
			"and a ceiling compared against zero is a gate measuring nothing", mcpInfoPath)
	}
	if info.Totals.Tools > publishedToolCeiling {
		t.Errorf("this server publishes %d tools, over its ceiling of %d.\n"+
			"  A client spends its tool budget across every configured server — %d in total — so "+
			"this leaves a user %d for everything else they connect.\n"+
			"  Adding one is a decision about somebody else's budget, not a local change. Either "+
			"drop a verb, or raise publishedToolCeiling in the same pull request and say what the "+
			"user gives up for it.\n"+
			"  The count reached %d by accretion with nothing counting it, which is what this gate "+
			"is here to stop happening again.",
			info.Totals.Tools, publishedToolCeiling,
			clientToolCeiling, clientToolCeiling-info.Totals.Tools, info.Totals.Tools)
	}
	t.Logf("published MCP surface: %d tools of a client's %d-tool budget (%d%%), ceiling %d; %d resources",
		info.Totals.Tools, clientToolCeiling, info.Totals.Tools*100/clientToolCeiling,
		publishedToolCeiling, info.Totals.Resources)
}
