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
//
// Derived from the generated file because the enum has no runtime
// enumeration — `Valid()` is a switch, which answers about a value you already
// hold and cannot list the ones you do not. Scanning the declarations is the
// only way to ask "what is the whole set" from a test, and asking that is the
// point: the failure this guards against is a source nobody classified.
func everyReachSource(t *testing.T) []crmcontracts.WorklistItemSource {
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
		if !ok || named.Name != "WorklistReachSource" {
			return true
		}
		lit, ok := spec.Values[0].(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING {
			return true
		}
		word, err := strconv.Unquote(lit.Value)
		if err != nil {
			t.Fatalf("a reach source constant that is not a string: %s", lit.Value)
		}
		out = append(out, crmcontracts.WorklistItemSource(word))
		return true
	})
	if len(out) == 0 {
		t.Fatal("no reach sources were found in the generated contract, so this test certifies " +
			"nothing — the scan is reading the wrong file or the wrong shape")
	}
	return out
}

// Every source says whether `all` widened it, and none of them says so by
// default: a source missing from the map reads as `false`, so the key set must
// EQUAL the contract's vocabulary rather than merely contain it.
func TestEveryWorklistSourceSaysWhetherItWidens(t *testing.T) {
	t.Parallel()
	known := map[crmcontracts.WorklistItemSource]bool{}
	for _, source := range everyReachSource(t) {
		known[source] = true
		if _, ok := sourceAnswersForTheActorOnly[source]; !ok {
			t.Errorf("%q is a source a reader can be told about and it says nothing about whether "+
				"`all` widened it — an unlisted source reads as widened, which is a claim about "+
				"this producer that nobody made", source)
		}
	}
	for source := range sourceAnswersForTheActorOnly {
		if !known[source] {
			t.Errorf("%q is classified here but the contract no longer offers it — a key matching "+
				"nothing ratifies a producer that is gone", source)
		}
	}
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
		"bounce":               "Bounces: the reader's own sends, per-user like the health lanes",
		"undelivered":          "Undelivered: per-user like the health lanes beside it",
		"domain_question":      "DomainQuestions: the reader's OWN undecided domains, the seam takes no owner argument",
		"weekly_commitment":    "Commitments: one promise this rep made",
		"failed_approval":      "FailedEffects: the decisions the acting rep approved",
		"introduction_request": "Introductions: per-user like Notices, the ask names one colleague",
	}
	for source, why := range perUser {
		if !sourceAnswersForTheActorOnly[source] {
			t.Errorf("%q is not marked as answering for the actor only, but its port says it is — %s",
				source, why)
		}
	}
	for source, personal := range sourceAnswersForTheActorOnly {
		if personal && perUser[source] == "" {
			t.Errorf("%q is marked as answering for the actor only with no port declaration behind "+
				"it here — name the port that declares it, or let it widen", source)
		}
	}
}
