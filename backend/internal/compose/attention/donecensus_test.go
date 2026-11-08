// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"gopkg.in/yaml.v3"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leavesByItsOwnVerb names, for each source that offers no Done, the verbs
// that take its row off the list. Each verb must be one the client performs
// for that source (performedBySource), or the way off is a button nobody can press.
var leavesByItsOwnVerb = map[crmcontracts.WorklistItemSource][]crmcontracts.AttentionItemActions{
	crmcontracts.WorklistItemSourceApproval:          {"decide"},
	crmcontracts.WorklistItemSourceBriefItem:         {"act", "set_aside", "dismiss"},
	crmcontracts.WorklistItemSourceConversationClaim: {"complete"},
	crmcontracts.WorklistItemSourceTask:              {"complete"},
	crmcontracts.WorklistItemSourceDealSuggestion:    {"decide", "dismiss"},
	crmcontracts.WorklistItemSourceTagSuggestion:     {"decide", "dismiss"},
	crmcontracts.WorklistItemSourceDedupeCandidate:   {"merge", "dismiss"},
	crmcontracts.WorklistItemSourceMeetingOutcome:    {"decide"},
	crmcontracts.WorklistItemSourceNotice:            {"acknowledge"},
	crmcontracts.WorklistItemSourceRelationshipDecay: {"dismiss"},
}

// leavesElsewhere names the sources whose way off the list is not a verb this
// census can see on the row, each with the reason.
// gatekit:fixture each source's way off the list, expected data and not a cost.
var leavesElsewhere = map[crmcontracts.WorklistItemSource]string{
	crmcontracts.WorklistItemSourceWeeklyCommitment: "the plan row is settled through the weekly plan's own state endpoint",
	crmcontracts.WorklistItemSourceFailedApproval:   "the reader acknowledges the failure on the row, which clears it for them alone",
}

// Every row kind the contract declares has a way off the reader's list. That is
// Done, a verb of its own the client can press, or a stated reason. The sources are
// read from the generated contract, so a new one fails here until it is placed.
func TestEverySourceOffersAWayOffTheList(t *testing.T) {
	t.Parallel()
	declared := declaredWorklistSources(t)
	reader := ids.NewV7()
	for _, source := range declared {
		probe := ranked{item: crmcontracts.WorklistItem{Source: source}}
		done := slices.Contains(dispositionsFor(probe, reader), disposeHandled)
		verbs, ownVerb := leavesByItsOwnVerb[source]
		reason, elsewhere := leavesElsewhere[source]
		placed := 0
		for _, way := range []bool{done, ownVerb, elsewhere} {
			if way {
				placed++
			}
		}
		switch {
		case placed == 0:
			t.Errorf("a %q row has no way off the list: offer Done in doneInPlace, name the verb "+
				"that clears it in leavesByItsOwnVerb, or say in leavesElsewhere why it cannot", source)
		case placed > 1:
			t.Errorf("%q is placed %d ways; keep the one that is true, so the census says one thing", source, placed)
		case elsewhere && reason == "":
			t.Errorf("%q is in leavesElsewhere with no reason", source)
		}
		for _, verb := range verbs {
			if !slices.Contains(performedBySource[string(source)], verb) {
				t.Errorf("%q leaves by %q, which the client does not perform for it (performedBySource)", source, verb)
			}
		}
	}
	// And every key names a declared source, so a renamed lane cannot leave a
	// stale entry that answers for nothing.
	for source := range leavesByItsOwnVerb {
		if !slices.Contains(declared, source) {
			t.Errorf("leavesByItsOwnVerb names %q, which the contract does not declare", source)
		}
	}
	for source := range leavesElsewhere {
		if !slices.Contains(declared, source) {
			t.Errorf("leavesElsewhere names %q, which the contract does not declare", source)
		}
	}
}

// A task or promise the reader may read but not change offers no Done, and
// says why instead of drawing an empty row.
func TestARowTheReaderCannotChangeSaysSo(t *testing.T) {
	t.Parallel()
	day := crmcontracts.Attention{
		AsOf: rankInstant,
		Planned: []crmcontracts.AttentionItem{
			item("theirs", "task", withDue(rankInstant.Add(48*time.Hour))),
			item("mine", "task", withDue(rankInstant.Add(48*time.Hour)), withActions("complete", "snooze")),
		},
		Commitments: lane(
			item("their-promise", "conversation_claim", withDue(rankInstant.Add(48*time.Hour)), withActions("open")),
			item("my-promise", "conversation_claim", withDue(rankInstant.Add(48*time.Hour)), withActions("complete", "open")),
		),
	}
	out := (&Service{}).worklistFrom(t.Context(), day, scopeAll, "", 50, waitingRead{}, worklistCursor{}, nil)
	want := map[string]bool{"theirs": true, "mine": false, "their-promise": true, "my-promise": false}
	for _, row := range out.Queue {
		readOnly, asked := want[row.Id]
		if !asked {
			continue
		}
		delete(want, row.Id)
		says := slices.ContainsFunc(row.Because, func(r crmcontracts.WorklistReason) bool {
			return r.Kind == "read_only"
		})
		if says != readOnly {
			t.Errorf("row %q says read_only = %v, want %v", row.Id, says, readOnly)
		}
	}
	if len(want) != 0 {
		t.Fatalf("the page drew none of %v, so this test judged nothing about them", want)
	}
}

// declaredWorklistSources is the WorklistItem source enum as crm.yaml declares
// it, and it fails unless the generated constants name the same set.
func declaredWorklistSources(t *testing.T) []crmcontracts.WorklistItemSource {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "api", "crm.yaml"))
	if err != nil {
		t.Fatalf("reading the contract: %v", err)
	}
	var contract struct {
		Components struct {
			Schemas map[string]struct {
				Properties map[string]struct {
					Enum []string `yaml:"enum"`
				} `yaml:"properties"`
			} `yaml:"schemas"`
		} `yaml:"components"`
	}
	if err := yaml.Unmarshal(raw, &contract); err != nil {
		t.Fatalf("parsing the contract: %v", err)
	}
	declared := contract.Components.Schemas["WorklistItem"].Properties["source"].Enum
	generated := generatedWorklistSources(t)
	slices.Sort(declared)
	slices.Sort(generated)
	if len(declared) == 0 || !slices.Equal(declared, generated) {
		t.Fatalf("crm.yaml declares sources %v and the generated contract %v: run make gen", declared, generated)
	}
	out := make([]crmcontracts.WorklistItemSource, 0, len(declared))
	for _, source := range declared {
		out = append(out, crmcontracts.WorklistItemSource(source))
	}
	return out
}

// generatedWorklistSources reads every WorklistItemSource constant from the
// generated contract.
func generatedWorklistSources(t *testing.T) []string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(),
		filepath.Join("..", "..", "contracts", "api_gen.go"), nil, parser.SkipObjectResolution)
	if err != nil {
		t.Fatalf("parsing the generated contract: %v", err)
	}
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		spec, ok := node.(*ast.ValueSpec)
		if !ok {
			return true
		}
		typed, ok := spec.Type.(*ast.Ident)
		if !ok || typed.Name != "WorklistItemSource" || len(spec.Values) != 1 {
			return true
		}
		literal, ok := spec.Values[0].(*ast.BasicLit)
		if !ok {
			return true
		}
		value, err := strconv.Unquote(literal.Value)
		if err != nil {
			t.Fatalf("an unreadable source constant %s: %v", literal.Value, err)
		}
		out = append(out, value)
		return true
	})
	return out
}
