// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind reachability H2

package backendarch

// Erasing a person and anonymizing one are the same act with one difference:
// the erased subject goes on a suppression list, and the anonymized subject may
// lawfully return. Everything else — which tables stop naming them, which
// payloads stop holding their words — has to be identical, because the promise
// made to the subject is the same promise.
//
// THE TWO ACTS DO NOT AGREE TODAY. This test does not assert they do: it records
// each difference that exists, with what that difference COSTS a subject, and
// fails when a NEW one appears in either direction. The ratifications below are
// the state of the gap, not a claim there isn't one.
//
// Both sets are computed from the call graph rather than listed. A list would be
// a third thing to keep current, and the next table added to one act would
// diverge from the other the same day it was added.
//
// THE SUBJECT IS THE TABLES each act writes — not the columns and not the
// predicates. Two acts can clear one table to different depths and this cannot
// see that. What it does see is a table one act touches and the other has never
// heard of.

import (
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/gradionhq/margince/backend/internal/shared/gatekit"
)

const (
	privacyPackage = "internal/modules/privacy"
	eraseRoot      = "Eraser.ErasePerson"
	anonymizeRoot  = "anonymizePersonRecord"
)

// clearedOnlyByTheEraser are tables the erase touches and the anonymize does
// not, ratified one at a time.
//
// The suppression list is the ONE difference the two acts are MEANT to have.
// Every entry below is therefore a defect, not a design: a table an anonymized
// subject's data survives in after an operator was told the action did what an
// erase does minus that list.
//
// They are ratified rather than fixed because each is a decision about what a
// returning subject may keep, and several need a ruling rather than a patch —
// the eraser is an ordered orchestration with legal-hold refusal, subject-key
// locking and statutory floors, not a scrub the anonymize can simply call.
//
// What this list buys is that the divergence is now WRITTEN DOWN, per table,
// and that a NEW one fails. #2205 carries the rulings.
var clearedOnlyByTheEraser = gatekit.Waive(map[string]string{
	"erasure_suppression":         "THE difference the two acts are meant to have. An erased subject's identifiers are hashed onto the suppression list so a later capture cannot re-create them; an anonymized subject may lawfully return, so writing one would refuse a person the product is allowed to know again.",
	"activity_retention_evidence": "records WHY the statutory correspondence floor held a row back from an erasure. The anonymize applies no floor, so it holds nothing back and has nothing to record — an absence produced by the floor difference, not data surviving.",
	"activity":                    "the subject's own words in an activity's content survive an anonymize; the eraser redacts them under the statutory correspondence floor. Anonymize applies no floor at all, so it is not that it keeps less — it never asked.",
	"agent_run":                   "an anonymized subject's agent run rows survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"ai_call_payload":             "an anonymized subject's stored AI call payloads, which can contain the prompt text naming them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"approval":                    "an anonymized subject's staged approvals naming them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"attachment":                  "an anonymized subject's attachment rows, including files they sent survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"capture_trace":               "an anonymized subject's capture traces, which record what was seen and when survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"comms_outbound":              "an anonymized subject's outbound message rows, which carry what was sent to them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_engagement":        "an anonymized subject's deal-room engagement records survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_participant":       "an anonymized subject's deal-room participant seats survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_session":           "an anonymized subject's deal-room sessions survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"lead":                        "the lead row is anonymized by the eraser's own lead pass; the person/anonymize action does not reach it, so a subject who exists as both a person and a lead is anonymized as one and not the other. That split is the finding, not a decision.",
	"lead_manual_signal":          "an anonymized subject's manual lead signals, which are notes a colleague wrote about them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"lead_score_history":          "an anonymized subject's lead score history survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"preference_token":            "an anonymized subject's preference tokens — the links that let them manage their own consent survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"raw_capture":                 "an anonymized subject's original captured payloads, the unparsed message bodies a capture was built from survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"scheduled_send":              "an anonymized subject's queued sends still addressed to them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"transcript_read":             "an anonymized subject's meeting-transcript readings survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"workflow_run":                "an anonymized subject's workflow run rows survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
})

// clearedOnlyByTheAnonymize are tables the anonymize touches and the erase does
// not. An erase that leaves what an anonymize removes is the worse direction, so
// these carry the same burden.
var clearedOnlyByTheAnonymize = gatekit.Waive(map[string]string{})

func TestErasingAndAnonymizingClearTheSameTables(t *testing.T) {
	defer clearedOnlyByTheEraser.AssertAllMatched(t)
	defer clearedOnlyByTheAnonymize.AssertAllMatched(t)

	graph := privacyCallGraph(t)
	erased := tablesReachableFrom(graph, eraseRoot)
	anonymized := tablesReachableFrom(graph, anonymizeRoot)

	// A walk that reached neither act is judging nothing.
	if len(erased) == 0 || len(anonymized) == 0 {
		t.Fatalf("the census reached %d tables from %s and %d from %s — it is not reading the package",
			len(erased), eraseRoot, len(anonymized), anonymizeRoot)
	}

	var eraserOnly, anonymizeOnly []string
	for _, table := range erased {
		if !slices.Contains(anonymized, table) && !clearedOnlyByTheEraser.Waived(t, table) {
			eraserOnly = append(eraserOnly, table)
		}
	}
	for _, table := range anonymized {
		if !slices.Contains(erased, table) && !clearedOnlyByTheAnonymize.Waived(t, table) {
			anonymizeOnly = append(anonymizeOnly, table)
		}
	}
	sort.Strings(eraserOnly)
	sort.Strings(anonymizeOnly)

	if len(eraserOnly) > 0 {
		t.Errorf("%d table(s) are cleared when a person is ERASED and not when they are ANONYMIZED.\n\n"+
			"Both acts make the subject unfindable; only the suppression list should differ. A table "+
			"here holds the subject's data after an operator was told it had been anonymized. Clear "+
			"it in both, or ratify it in clearedOnlyByTheEraser with the reason a returning subject "+
			"may keep it.\n\n\t%s", len(eraserOnly), strings.Join(eraserOnly, "\n\t"))
	}
	if len(anonymizeOnly) > 0 {
		t.Errorf("%d table(s) are cleared when a person is ANONYMIZED and not when they are ERASED.\n\n"+
			"This is the worse direction: an ERASED subject's data survives where an anonymized "+
			"subject's does not.\n\n\t%s", len(anonymizeOnly), strings.Join(anonymizeOnly, "\n\t"))
	}
}

// privacyFunc is one function: the tables it writes itself, and the functions it
// calls that this census can resolve.
type privacyFunc struct {
	tables map[string]bool
	calls  map[string]bool
}

// privacyCallGraph is the shared package graph with this census's question
// asked of it: which TABLES each function's reachable statements write.
//
// The graph itself — receiver-type keying, package-level statements attributed
// to whoever names them, the unresolvable edges it will not follow — is in
// callgraph_test.go, shared with the organization rename census. What a
// statement MEANS stays here, because it is not the same question over there.
func privacyCallGraph(t *testing.T) map[string]*privacyFunc {
	t.Helper()
	graph := packageCallGraph(t, privacyPackage)
	out := map[string]*privacyFunc{}
	for name, entry := range graph {
		tables := map[string]bool{}
		for _, statement := range entry.statements {
			for _, table := range sqlWriteTargets(statement) {
				tables[table] = true
			}
		}
		out[name] = &privacyFunc{tables: tables, calls: entry.calls}
	}
	return out
}

func tablesReachableFrom(graph map[string]*privacyFunc, root string) []string {
	seen, found := map[string]bool{}, map[string]bool{}
	var walk func(string)
	walk = func(name string) {
		if seen[name] {
			return
		}
		seen[name] = true
		entry, known := graph[name]
		if !known {
			return
		}
		for table := range entry.tables {
			found[table] = true
		}
		for called := range entry.calls {
			walk(called)
		}
	}
	walk(root)
	tables := make([]string, 0, len(found))
	for table := range found {
		tables = append(tables, table)
	}
	sort.Strings(tables)
	return tables
}
