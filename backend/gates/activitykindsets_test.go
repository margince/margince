// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// The two activity-kind sets relstrength holds, against the vocabulary the
// contract actually publishes.
//
// It is a gate rather than a test beside the package because the answer has to
// be derived from `api/crm.yaml`. A list of kinds typed out in relstrength's own
// suite grows only when somebody remembers to grow it, so a seventh kind added
// to the contract would leave every assertion below passing over a smaller
// vocabulary — the census that fails short and reports PASS.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/relstrength"
)

// publishedActivityKinds reads the contract's ActivityKind values off the
// GENERATED constants rather than taking them typed out here.
//
// The generated file is the derivation, not a second copy: `api/crm.yaml`
// declares this vocabulary inline at several call sites rather than as one named
// schema, so there is no single YAML node to read — but the generator collapses
// them into one type, and the drift gate already fails if that type stops
// matching the contract. Reading it here is therefore reading crm.yaml one hop
// away, and a seventh kind arrives in this list without anybody editing it.
func publishedActivityKinds(t *testing.T) []string {
	t.Helper()
	const generated = "internal/contracts/api_gen.go"
	file, err := parser.ParseFile(token.NewFileSet(), generated, nil, 0)
	if err != nil {
		t.Fatalf("reading the generated contract: %v", err)
	}
	var kinds []string
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Values) != 1 {
				continue
			}
			named, isNamed := value.Type.(*ast.Ident)
			if !isNamed || named.Name != "ActivityKind" {
				continue
			}
			if kind, isString := gatekit.LiteralText(value.Values[0]); isString {
				kinds = append(kinds, kind)
			}
		}
	}
	return kinds
}

// neitherSet records each contract kind that belongs to no set, with why. It is
// what makes a NEW kind fail: an unlisted one is neither classified nor
// deliberately excluded, and nobody has decided which it is.
var neitherSet = gatekit.Waive(map[string]string{
	"note": "a record of one person's thinking. Nobody was in the room and nothing was exchanged, so it has neither participants nor warmth — an unlinked note is a workspace-shared thought",
	"task": "one person's intent. Counting it would let a rep's own to-do list score as a relationship, and stamping participants on it would name people who were never told",
})

// Every kind the contract publishes is either in the participant set or
// deliberately in neither, and this is the assertion a new kind trips.
func TestEveryPublishedActivityKindIsClassified(t *testing.T) {
	t.Parallel()
	defer neitherSet.AssertAllMatched(t)
	kinds := publishedActivityKinds(t)
	if len(kinds) < 4 {
		t.Fatalf("the contract publishes %d activity kinds; this gate would judge almost nothing", len(kinds))
	}
	for _, kind := range kinds {
		if relstrength.IsParticipantKind(kind) {
			continue
		}
		if neitherSet.Waived(t, kind) {
			continue
		}
		t.Errorf("activity kind %q is in no relstrength set and in no exclusion — decide whether it is meaningful to ask who was on one (relstrength.participantKinds) and whether it means two people spoke (interactionKinds), and record the answer", kind)
	}
}

// THE DIRECTION THE TWO SETS MAY DIFFER IN, and only that direction. A kind may
// be worth recording the people on without being worth scoring — a group chat
// is exactly that — while a kind scored as a relationship with nobody recorded
// on it would leave the interaction graph unable to say who the relationship is
// with.
func TestEveryScoredKindHasParticipants(t *testing.T) {
	t.Parallel()
	for _, kind := range publishedActivityKinds(t) {
		if scoredKind(t, kind) && !relstrength.IsParticipantKind(kind) {
			t.Errorf("%q is scored as an interaction but records no participants — the graph would hold a relationship with nobody in it", kind)
		}
	}
}

// A message has a room AND counts as warmth, and it is the unit that makes the
// second half safe. All three are asserted because each is a decision somebody
// could undo without noticing the others: dropping it from the scoring set puts
// a chat-only account back to reading as never having spoken, dropping it from
// the participant set puts a group chat back to naming nobody who was in it,
// and counting its rows instead of its conversation-days lets an afternoon of
// one-line replies outweigh a quarter of meetings.
func TestAMessageHasParticipantsAndIsScoredByTheConversationDay(t *testing.T) {
	t.Parallel()
	if !relstrength.IsParticipantKind("message") {
		t.Error("a message records no participants — a group chat names everyone who was in it, and this is where that is kept")
	}
	if !scoredKind(t, "message") {
		t.Error("a message does not score as an interaction — two people spoke, which is the membership test the scoring set applies, and an account whose whole relationship runs over a channel reads as having none")
	}
	unit := relstrength.InteractionUnitSQL("a")
	if !strings.Contains(unit, "'message'") || !strings.Contains(unit, "a.thread_key") {
		t.Errorf("the counting unit is %q, which does not separate a message from the kinds counted per row — the scoring set holds a kind that arrives dozens of rows to a conversation", unit)
	}
}

// scoredKind asks the SQL RENDERING, because that is what actually decides: the
// scoring set has no Go predicate left, only the three queries that read it, and
// a gate asking a different question than the queries do would pass while they
// disagreed.
func scoredKind(t *testing.T, kind string) bool {
	t.Helper()
	list := relstrength.InteractionKindSQLList()
	if !strings.HasPrefix(list, "'") || !strings.HasSuffix(list, "'") {
		t.Fatalf("the scoring list renders as %q, which is not a quoted SQL list — this gate would judge nothing", list)
	}
	for _, got := range strings.Split(strings.Trim(list, "'"), "','") {
		if got == kind {
			return true
		}
	}
	return false
}
