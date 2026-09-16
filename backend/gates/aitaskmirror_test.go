// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// api/ai-tasks.yaml is a MIRROR, and until this gate nothing failed when it
// stopped being one.
//
// The file's own header states the relationship: the AI task contract is
// normative for the task list, the tiers, each task's fallback ladder, its
// execution mode and its budget posture, and this repository's copy "must match
// it verbatim". Editing a ladder here is a routing-POLICY change that is
// supposed to require a spec decision — "never a silent code diff".
//
// The existing drift gate proves the generated Go matches this file. Nothing
// proved this file matches the document it mirrors, and by the time #1277 was
// filed it had drifted four times, each addition shipping without anything
// failing.
//
// WHAT THIS GATE CAN AND CANNOT DO. It cannot read the upstream document: that
// lives outside this tree and a public contributor could not fetch it either.
// What it can do is make an unreconciled declaration IMPOSSIBLE TO ADD
// SILENTLY. Every task and tier below is ratified as either mirrored upstream
// or knowingly divergent, and a declaration in neither set fails this test. The
// author of the fifth divergence then has to choose between reconciling the
// document and writing down that they did not — which is the choice the mirror
// relationship was always asking for and never enforcing.
//
// So this does not close #1277's first half. Reconciliation is somebody's
// editorial pass over a document this repository does not hold. It closes the
// second half, which the issue calls "the one that matters": the reason it
// drifted four times is that disagreement was nobody's failing test.

import (
	"os"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const aiTasksPath = "api/ai-tasks.yaml"

// mirroredTasks are the task declarations believed to exist upstream verbatim.
//
// "Believed" is the honest word and the limit of what a build-side gate can
// say: nothing here reads the document. What each entry records is that a human
// reconciled this name, which is the fact the next author needs and the one
// that was missing.
var mirroredTasks = gatekit.Waive(map[string]string{
	"capture_classify":                "attention routing for captured mail — the first task declared and mirrored with the original contract",
	"owed_verdict":                    "whether a captured message leaves us owing a reply",
	"request_settlement":              "whether an inbound message settles a request we made",
	"capture_counterparty_verdict":    "whether a captured counterparty is a business one",
	"capture_confidentiality_verdict": "whether a captured thread is confidential",
	"stage_evidence_extract":          "the evidence a stage move rests on",
	"transcript_propose":              "proposals read out of a meeting transcript",
	"propose_roles":                   "who plays which part on a deal",
	"enrich":                          "third-party enrichment of a record",
	"summarize":                       "the summary of one record or thread",
	"draft_reply":                     "a reply drafted for a human to send",
	"nl_search":                       "a natural-language query over the workspace",
	"cold_start":                      "the first pass over a newly connected installation",
	"transcript":                      "the transcript itself",
	"corpus_ask":                      "a question answered from a document corpus",
	"account_scan":                    "the account-level scan",
	"deal_health":                     "the health verdict on a deal",
	"weekly_review":                   "the weekly review's prose",
	"weekly_learnings":                "what the week taught",
	"brief_ranking":                   "the morning Brief's ranking",
	"agent_loop":                      "the agent's own loop",
	"offer_draft":                     "an offer drafted from a deal",
	"document_extract":                "fields read out of an attached document",
	"site_extract":                    "fields read off a company website",
	"site_fact_extract":               "one fact read off a page",
	"cert_judge":                      "the certification judge",
	"voice_build":                     "building a voice profile",
	"rate_extract":                    "an FX rate read from a source",
})

// divergentTasks are declared HERE and known to be absent upstream — the four
// #1277 names, minus the one #1254 added deliberately.
//
// Each entry names the issue that will reconcile it. They are ratified rather
// than removed because the binary is built from this copy and is complete: the
// gap is governance, not correctness, and deleting a shipped task to satisfy a
// gate would be the one change here that broke something.
var divergentTasks = gatekit.Waive(map[string]string{
	"growth_fit":     "shipped with the growth-fit feature and never reconciled upstream. #1277 half one",
	"signal_extract": "shipped with material-event extraction and never reconciled upstream. #1277 half one",
	"site_triage":    "shipped with the site-triage feature and never reconciled upstream. #1277 half one",
})

// mirroredTiers and divergentTiers are the same pair for the tier vocabulary,
// which the contract governs alongside the tasks: a tier is what a ladder's
// rungs are named from, so one added here and not upstream widens the routing
// vocabulary the document is supposed to be the authority for.
var mirroredTiers = gatekit.Waive(map[string]string{
	"local_small": "the cheapest rung, and every ladder's floor",
	"cheap_cloud": "the first cloud rung",
	"premium":     "the strong general rung",
	"local_large": "the local rung a deployment runs itself",
})

var divergentTiers = gatekit.Waive(map[string]string{
	"frontier": "a tier, not a task, and the instance #1042 was filed for before being folded into #1277. No workload selects it, so reconciling it is a vocabulary addition rather than a routing-policy change. #1277 half one",
})

type aiTaskDoc struct {
	Tiers []string             `yaml:"tiers"`
	Tasks map[string]yaml.Node `yaml:"tasks"`
	Embed map[string]yaml.Node `yaml:"embed"`
}

func TestEveryAITaskDeclarationIsRatifiedAgainstTheMirror(t *testing.T) {
	t.Parallel()
	raw, err := os.ReadFile(aiTasksPath)
	if err != nil {
		t.Fatalf("reading %s: %v", aiTasksPath, err)
	}
	var doc aiTaskDoc
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parsing %s: %v", aiTasksPath, err)
	}
	if len(doc.Tasks) == 0 || len(doc.Tiers) == 0 {
		t.Fatalf("%s declared %d tasks and %d tiers — a mirror that parses to nothing "+
			"certifies nothing, so this is a parse failure wearing a pass",
			aiTasksPath, len(doc.Tasks), len(doc.Tiers))
	}

	ratify(t, "task", sortedTaskNames(doc.Tasks), mirroredTasks, divergentTasks,
		"a task declared here is a routing-policy decision the contract is meant to own")
	ratify(t, "tier", doc.Tiers, mirroredTiers, divergentTiers,
		"a tier is what a ladder's rungs are named from, so adding one widens the routing vocabulary")

	t.Logf("ai-tasks mirror: %d tasks (%d divergent), %d tiers (%d divergent)",
		len(doc.Tasks), len(divergentTasks.Subjects()), len(doc.Tiers), len(divergentTiers.Subjects()))
	mirroredTasks.AssertAllMatched(t)
	divergentTasks.AssertAllMatched(t)
	mirroredTiers.AssertAllMatched(t)
	divergentTiers.AssertAllMatched(t)
}

// ratify fails any declaration that is in neither set, and any that is in both.
func ratify(t *testing.T, kind string, declared []string, mirrored, divergent *gatekit.Waivers[string], why string) {
	t.Helper()
	for _, name := range declared {
		inMirror := mirrored.Waived(t, name)
		inDivergent := divergent.Waived(t, name)
		switch {
		case inMirror && inDivergent:
			t.Errorf("%s %q is ratified as BOTH mirrored and divergent — it is one or the other, "+
				"and a reader cannot tell which document to trust", kind, name)
		case inMirror || inDivergent:
		default:
			t.Errorf("%s %q is declared in %s and ratified in neither set.\n"+
				"  %s, and this file is a MIRROR of the document that owns it.\n"+
				"  Reconcile it upstream and add it to mirrored%ss, or record the divergence in "+
				"divergent%ss with the issue that will close it.\n"+
				"  Left alone it is the fifth silent drift, which is what #1277 was filed for.",
				kind, name, aiTasksPath, why, setName(kind), setName(kind))
		}
	}
}

// sortedTaskNames is the declared task names in a stable order, so two runs
// report the same first failure.
// setName spells a kind the way the two ratification sets are named, so the
// failure prints a symbol a reader can grep for.
func setName(kind string) string { return strings.ToUpper(kind[:1]) + kind[1:] }

func sortedTaskNames(m map[string]yaml.Node) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
