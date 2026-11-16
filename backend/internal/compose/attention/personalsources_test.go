// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// contractsSourceFile is the generated vocabulary this map answers to. Read
// rather than retyped: a list written here would be a second copy of the enum,
// and the copy that goes stale is the one nothing fails on.
const contractsSourceFile = "../../contracts/api_gen.go"

// everyReachSource reads the contract's reach vocabulary out of the generated
// constants.
func everyReachSource(t *testing.T) []crmcontracts.WorklistItemSource {
	t.Helper()
	return contractEnum(t, "WorklistReachSource")
}

// contractEnum lists the string constants of one generated enum type. The enum
// has no runtime list (`Valid()` is a switch), so the declarations are scanned.
func contractEnum(t *testing.T, typeName string) []crmcontracts.WorklistItemSource {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, contractsSourceFile, nil, 0)
	if err != nil {
		t.Fatalf("reading the generated contract vocabulary: %v", err)
	}
	var out []crmcontracts.WorklistItemSource
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok || len(spec.Names) != 1 || len(spec.Values) != 1 {
			return true
		}
		named, ok := spec.Type.(*ast.Ident)
		if !ok || named.Name != typeName {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		word, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("a %s constant that is not a string: %s", typeName, lit.Value)
		}
		out = append(out, crmcontracts.WorklistItemSource(word))
		return true
	})
	if len(out) == 0 {
		t.Fatalf("no %s values were found in the generated contract, so this test certifies "+
			"nothing — the scan is reading the wrong file or the wrong shape", typeName)
	}
	return out
}

// The per-user sources are the ones their own ports declare per-user.
//
// Written out rather than derived, because the fact lives in prose on each port
// and no signature carries it: Approvals and Duplicates take no owner argument
// either and widen perfectly well through row scope. So this pins the reading
// against the sentence it came from, and a port that changes its mind fails
// here rather than quietly telling a manager that a source widened.
func TestThePerUserSourcesAreTheOnesTheirPortsDeclare(t *testing.T) {
	t.Parallel()
	perUser := map[crmcontracts.WorklistItemSource]string{
		"notice":               "Notices: the acting contact's own unread notices",
		"capture_health":       "CaptureHealth: capture is per-user, the seam refuses a principal with no human behind it",
		"ai_work_health":       "AIWork: per-user like CaptureHealth",
		"booking_health":       "BookingHealth: the reader's own booking page, refused for a principal with no human",
		"bounce":               "Bounces: the reader's own sends, per-user like the health lanes",
		"undelivered":          "Undelivered: per-user like the health lanes beside it",
		"domain_question":      "DomainQuestions: the reader's OWN undecided domains, the seam takes no owner argument",
		"weekly_commitment":    "Commitments: one promise this rep made",
		"failed_approval":      "FailedEffects: the decisions the acting rep approved",
		"introduction_request": "Introductions: per-user like Notices, the ask names one colleague",
		"awaiting_reply":       "Awaiting: the reader's own sends, read only on their own day",
		"meeting_follow_up":    "Awaiting: the reader's own meetings, read only on their own day",
	}
	for source, why := range perUser {
		if !answersForTheActorOnly(source) {
			t.Errorf("%q is not marked as answering for the actor only, but its port says it is — %s",
				source, why)
		}
	}
	for source, kind := range worklistKinds {
		if kind.personal && perUser[source] == "" {
			t.Errorf("%q is marked as answering for the actor only with no port declaration behind "+
				"it here — name the port that declares it, or let it widen", source)
		}
	}
}
