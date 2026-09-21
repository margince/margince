// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

//go:build !integration

package gates

// A 🟢 or 🟡 in an operation's SUMMARY is a claim about that operation's
// autonomy tier, and it has to be the tier the operation actually declares.
//
// The summary is the line with the widest reach of anything in the contract: it
// becomes the method comment in every generated client and the heading in the
// published docs, and it is the only sentence most readers will see of an
// operation they are deciding whether to hand an agent. `sendOffer` carried
// `🟡 — leaves the workspace` while declaring `x-agent-access: human-only` and
// performing no transport at all, so the most widely read line of it said both
// that an agent could stage it and that something left the installation. Neither
// was true, and the long description had already been corrected around it.
//
// The mark means what the contract's preamble says it means: 🟢 is
// `auto_execute`, 🟡 is `confirmation_required`, and both belong to an
// `x-mcp-tool` tier. An operation with no tier — every `x-agent-access` verb —
// has no mark to write.
//
// THE OTHER USE IS REAL, and this is not an exception carved for it: an
// operation may describe the STAGED WORK IT CREATES rather than its own tier,
// and `listApprovals` is 🟢 itself while listing 🟡 actions. Those write the
// mark against the noun it qualifies — "staged 🟡 proposals", "🟡 actions
// awaiting human decision" — so the rule reads what the mark is attached to
// rather than merely that one is present.
//
// WHAT IT CANNOT SEE. A tier claim made in the long DESCRIPTION rather than the
// summary, and a claim spelled in words ("runs directly", "waits for a human")
// rather than with a mark. It holds the line that travels furthest, not every
// sentence that could be wrong.

import (
	"os"
	"regexp"
	"slices"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// stagedWorkNouns are what a mark may qualify when it is NOT about the
// operation's own tier. Each names work the operation stages or lists for a
// human to decide, which is a different subject from what the operation itself
// does when an agent calls it.
var stagedWorkNouns = []string{"proposal", "proposals", "action", "actions", "approval", "approvals"}

// tierMarkClaim finds a mark and captures the word it precedes, so a mark
// attached to staged work is told apart from one standing for the operation.
var tierMarkClaim = regexp.MustCompile(`(🟢|🟡)[^\p{L}]*(\p{L}+)?`)

func TestNoOperationSummaryClaimsATierItDoesNotHave(t *testing.T) {
	t.Parallel()
	ops := summarisedOperations(t)
	if len(ops) == 0 {
		t.Fatal("no operations were read out of the contract — this census reads an empty document, and an empty one reports PASS")
	}

	marked := 0
	for _, name := range sortedOperationNames(ops) {
		op := ops[name]
		for _, claim := range tierMarkClaim.FindAllStringSubmatch(op.Summary, -1) {
			mark, qualifies := claim[1], strings.ToLower(claim[2])
			if slices.Contains(stagedWorkNouns, qualifies) {
				continue
			}
			marked++
			want := markOfTier(op.MCPTool.Tier)
			if want == "" {
				t.Errorf("%s summarises itself as %s but declares no agent tier (x-mcp-tool), so there is no tier for the mark to be:\n\t%s\n\n"+
					"This is the line a generated client puts on the method and the docs put at the top. Say what the operation does, or declare the tier.",
					name, mark, op.Summary)
				continue
			}
			if mark != want {
				t.Errorf("%s summarises itself as %s but declares tier %q, which is %s:\n\t%s",
					name, mark, op.MCPTool.Tier, want, op.Summary)
			}
		}
	}
	if marked == 0 {
		t.Error("no summary in the contract makes a tier claim at all. Either every mark has moved into the staged-work form this gate deliberately leaves alone — in which case it now guards nothing and should say so — or the claim is no longer written this way and the pattern above has gone stale")
	}
}

// markOfTier renders a declared tier as the contract's preamble writes it, and
// answers empty for an operation that declares none.
func markOfTier(tier string) string {
	switch tier {
	case "auto_execute":
		return "🟢"
	case "confirmation_required":
		return "🟡"
	default:
		return ""
	}
}

// summarisedOperation is the part of an operation this gate judges.
type summarisedOperation struct {
	Summary string `yaml:"summary"`
	MCPTool struct {
		Tier string `yaml:"tier"`
	} `yaml:"x-mcp-tool"` //nolint:tagliatelle // the contract's vendor key, not ours to rename
	OperationID string `yaml:"operationId"` //nolint:tagliatelle // OpenAPI's key, not ours to rename
}

// summarisedOperations reads every operation in the contract, keyed by its id.
// Every HTTP method is read rather than a list of the ones that carry tiers
// today: a mark on a GET is as wrong as one on a POST, and a method this gate
// forgot would be a place the rule silently stopped applying.
func summarisedOperations(t *testing.T) map[string]summarisedOperation {
	t.Helper()
	raw, err := os.ReadFile("api/crm.yaml")
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var doc struct {
		Paths map[string]map[string]yaml.Node `yaml:"paths"`
	}
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	out := map[string]summarisedOperation{}
	for path, methods := range doc.Paths {
		for method, node := range methods {
			if method == "parameters" {
				continue
			}
			var op summarisedOperation
			if err := node.Decode(&op); err != nil {
				t.Fatalf("reading %s %s out of the contract: %v", strings.ToUpper(method), path, err)
			}
			if op.OperationID == "" {
				t.Errorf("%s %s declares no operationId, so this census cannot name what it judges", strings.ToUpper(method), path)
				continue
			}
			out[op.OperationID] = op
		}
	}
	return out
}

func sortedOperationNames(ops map[string]summarisedOperation) []string {
	names := make([]string, 0, len(ops))
	for name := range ops {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
