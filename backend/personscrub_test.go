// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// Erasing a person and anonymizing one are the same act with one difference:
// the erased subject goes on a suppression list, and the anonymized subject may
// lawfully return. Everything else — which tables stop naming them, which
// payloads stop holding their words — has to be identical, because the promise
// made to the subject is the same promise.
//
// `retentionactions.go` said so in a comment and nothing held it. THIS IS THE
// TEST THAT REPLACES THE COMMENT.
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
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
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

// writesATable matches the target of a statement that REMOVES or REWRITES a
// person's data. A SELECT names tables too and changes nothing about them.
var writesATable = regexp.MustCompile(`(?is)(?:DELETE\s+FROM|UPDATE)\s+([a-z_][a-z0-9_]*)`)

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
	"activity":                     "the subject's own words in an activity's content survive an anonymize; the eraser redacts them under the statutory correspondence floor. Anonymize applies no floor at all, so it is not that it keeps less — it never asked.",
	"agent_run":                    "an anonymized subject's agent run rows survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"ai_call_payload":              "an anonymized subject's stored AI call payloads, which can contain the prompt text naming them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"ai_feedback":                  "an anonymized subject's feedback rows, which name them as the subject an AI answer was judged about survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"approval":                     "an anonymized subject's staged approvals naming them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"attachment":                   "an anonymized subject's attachment rows, including files they sent survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"capture_pending_counterparty": "an anonymized subject's pending-counterparty rows, which hold an identifier waiting to be matched to a person survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"capture_trace":                "an anonymized subject's capture traces, which record what was seen and when survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"comms_outbound":               "an anonymized subject's outbound message rows, which carry what was sent to them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_engagement":         "an anonymized subject's deal-room engagement records survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_participant":        "an anonymized subject's deal-room participant seats survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"deal_room_session":            "an anonymized subject's deal-room sessions survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"field_provenance":             "an anonymized subject's record of WHICH SOURCE supplied each field value — the trail back to where their data came from survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"lead":                         "the lead row is anonymized by the eraser's own lead pass; the person/anonymize action does not reach it, so a subject who exists as both a person and a lead is anonymized as one and not the other. That split is the finding, not a decision.",
	"lead_manual_signal":           "an anonymized subject's manual lead signals, which are notes a colleague wrote about them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"lead_score_history":           "an anonymized subject's lead score history survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"preference_token":             "an anonymized subject's preference tokens — the links that let them manage their own consent survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"raw_capture":                  "an anonymized subject's original captured payloads, the unparsed message bodies a capture was built from survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"scheduled_send":               "an anonymized subject's queued sends still addressed to them survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"transcript_read":              "an anonymized subject's meeting-transcript readings survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
	"workflow_run":                 "an anonymized subject's workflow run rows survives the action. Recorded as the state this gate froze, not as a decision that it is right — the ruling is #2205.",
})

// clearedOnlyByTheAnonymize are tables the anonymize touches and the erase does
// not. An erase that leaves what an anonymize removes is the worse direction, so
// these carry the same burden.
var clearedOnlyByTheAnonymize = gatekit.Waive(map[string]string{
	"activity_participant": "the anonymize rewrites the participant row's stored display name in place. The eraser reaches the same row through its timeline redaction rather than by name, so this is one act naming a table the other reaches another way — the only difference in this direction, and the one that is not a gap.",
})

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

// privacyCallGraph reads the privacy package, keyed by RECEIVER TYPE and name.
//
// Bare names are not enough. `apply` is a method on the retention service and
// also a plausible name elsewhere; following calls by name alone walked from the
// eraser into the policy store and reported `retention_policy` as a table an
// erase clears, which it does not.
//
// So an edge is followed only when it can be resolved: a plain function, or a
// method called on the CALLER'S OWN receiver. A call on any other value is not
// followed, because nothing here knows that value's type.
//
// The narrowing is in the safe direction: an unfollowed edge hides a table from
// BOTH acts equally, so it cannot invent a difference between them. A table
// written through a helper in another package is invisible for the same reason.
func privacyCallGraph(t *testing.T) map[string]*privacyFunc {
	t.Helper()
	graph := map[string]*privacyFunc{}
	sources, err := filepath.Glob(filepath.Join(privacyPackage, "*.go"))
	if err != nil {
		t.Fatalf("listing %s: %v", privacyPackage, err)
	}
	fset := token.NewFileSet()
	for _, path := range sources {
		if strings.HasSuffix(path, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parsing %s: %v", path, parseErr)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			recvType, recvVar := receiverTypeName(fn), receiverVarName(fn)
			entry := &privacyFunc{tables: map[string]bool{}, calls: map[string]bool{}}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				// Statements go through this package's shared value reader, so
				// one written in double quotes with escapes decodes rather than
				// being matched as source text, and one assembled with `+` is
				// read whole.
				if expr, isExpr := node.(ast.Expr); isExpr {
					if statement, readable := stringValue(expr, nil); readable {
						for _, match := range writesATable.FindAllStringSubmatch(statement, -1) {
							entry.tables[match[1]] = true
						}
					}
				}
				call, isCall := node.(*ast.CallExpr)
				if !isCall {
					return true
				}
				switch fun := call.Fun.(type) {
				case *ast.Ident:
					entry.calls[fun.Name] = true
				case *ast.SelectorExpr:
					if base, isIdent := fun.X.(*ast.Ident); isIdent &&
						recvVar != "" && base.Name == recvVar {
						entry.calls[scrubKey(recvType, fun.Sel.Name)] = true
					}
				}
				return true
			})
			graph[scrubKey(recvType, fn.Name.Name)] = entry
		}
	}
	return graph
}

// scrubKey keys a method by its receiver type and a plain function by
// itself.
func scrubKey(receiver, name string) string {
	if receiver == "" {
		return name
	}
	return receiver + "." + name
}

// receiverVarName is what a method calls its own receiver, so a call on it can
// be told from a call on anything else. This package's receiverTypeName already
// gives the type; only the variable was missing.
func receiverVarName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 || len(fn.Recv.List[0].Names) == 0 {
		return ""
	}
	return fn.Recv.List[0].Names[0].Name
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
