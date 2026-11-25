// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every AI draft follows the workspace's team rules, and this is what keeps
// that true.
//
// The set of surfaces comes from the tree, like the shared-rules census. A
// production file that names draftrules.Shared or draftrules.TeamRules composes
// the team-rules paragraph, so it has to put the saved text into its user turn.
// A new drafter that composes either and skips the block fails here.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose/accountdraft"
	"github.com/margince/margince/backend/internal/compose/contactdraft"
	"github.com/margince/margince/backend/internal/compose/draftrules"
	"github.com/margince/margince/backend/internal/compose/draftvoice"
	"github.com/margince/margince/backend/internal/compose/rewrite"
	"github.com/margince/margince/backend/internal/shared/ports/model"
)

const teamRulesText = "Never quote a discount in writing. Write licence, never license."

// teamRulesRequests is the request each surface builds when the workspace saved
// rules, for a sender without a voice profile. Keys are the files that compose
// the paragraph. Each request comes from the surface's own builder, so a block
// that renders and never reaches the user turn fails.
func teamRulesRequests(t *testing.T, rules string) map[string]model.Request {
	t.Helper()
	voice := draftvoice.Context{TeamRules: rules}
	contact, err := contactdraft.GroundedRequest(contactdraft.Input{
		Recipient: contactdraft.RecipientIn{ID: "p1", Name: "Marek", FirstName: "Marek"},
	}, voice)
	if err != nil {
		t.Fatalf("the contact composer builds no request: %v", err)
	}
	account, err := accountdraft.GroundedRequest(accountdraft.Input{
		Company:   "Northwind",
		Recipient: accountdraft.RecipientIn{ID: "p1", Name: "Priya"},
	}, voice)
	if err != nil {
		t.Fatalf("the account composer builds no request: %v", err)
	}
	reply, err := replyDraftRequest(replyActivityData{Recipient: "Dietmar"}, rules, nil, "")
	if err != nil {
		t.Fatalf("the reply drafter builds no request: %v", err)
	}
	return map[string]model.Request{
		"contactdraft/write.go": contact,
		"accountdraft/write.go": account,
		"replydraftmodel.go":    reply,
		"rewrite/prompt.go": rewrite.Request(rewrite.Input{
			Envelope: rewrite.EnvelopeEmail, Verb: rewrite.VerbPolish, Body: "hello anna, can we talk pricing?",
		}, model.AboutNoContact("team rules parity"), voice),
		"offerdraft.go": offerDraftRequest([]dealContextItem{{SourceID: "activity:1", Snippet: "two seats"}}, nil, "en", rules),
	}
}

func TestEveryDraftSurfaceCarriesTheTeamRulesBlockWhenTheyAreSetAndNoneWhenTheyAreNot(t *testing.T) {
	set, unset := teamRulesRequests(t, teamRulesText), teamRulesRequests(t, "")
	for where, req := range set {
		sent := userTurn(t, where, req)
		if !fencedIn(t, sent, teamRulesText) {
			t.Errorf("%s does not send the team rules inside this call's fence, so a rep with no voice "+
				"profile writes outside them or they read as instructions", where)
		}
		if !strings.Contains(req.System, draftrules.TeamRules) {
			t.Errorf("%s sets the team rules but its system turn never ranks them over the sender's own rules", where)
		}
		if strings.Contains(req.System, teamRulesText) {
			t.Errorf("%s copies the team rules into the system turn, where they read as the product's own instruction", where)
		}
	}
	for where, req := range unset {
		if strings.Contains(userTurn(t, where, req), "Team rules:") {
			t.Errorf("%s sends a team rules block although the workspace saved none", where)
		}
	}
}

func TestTheTeamRulesParagraphRanksThemAboveTheSenderAndBelowTheFacts(t *testing.T) {
	for _, want := range []string{
		"outranks the sender's own rules and learned voice",
		"never allows a fact the data does not hold",
	} {
		if !strings.Contains(draftrules.TeamRules, want) {
			t.Errorf("the team rules paragraph no longer says %q", want)
		}
	}
	if !strings.Contains(draftrules.Shared, draftrules.TeamRules) {
		t.Error("the shared rules do not carry the team rules paragraph, so the three composers and the reply drafter would not either")
	}
	if !strings.Contains(draftvoice.SystemRule, "the team rules win") {
		t.Error("a voiced system turn no longer ranks the profile's human-authored rules below the team rules")
	}
}

// The requests above cover the same files that compose the paragraph.
func TestTheTeamRulesTableCoversEveryFileThatComposesTheParagraph(t *testing.T) {
	swept := filesComposingTheSharedRules(t, "TeamRules")
	if len(swept) == 0 {
		t.Fatal("no file under internal/compose composes the draft rules, so this gate is reading an empty tree")
	}
	built := teamRulesRequests(t, teamRulesText)
	for _, where := range swept {
		if _, covered := built[where]; !covered {
			t.Errorf("%s composes the draft rules but teamRulesRequests builds no request for it, so nothing "+
				"checks that the team rules reach the model it calls", where)
		}
	}
	for where := range built {
		if !slicesContains(swept, where) {
			t.Errorf("teamRulesRequests names %s, which no longer composes the draft rules: remove the row", where)
		}
	}
}
