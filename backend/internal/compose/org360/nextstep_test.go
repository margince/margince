// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package org360

import (
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The no-next-step rule used to hand the reader back their own problem: it had
// worked out that nothing is scheduled and which deals that is true of, and
// then asked them to "set the next step". It now names the step and prepares
// the body that writes it.
//
// What the reader ACCEPTS and what the button WRITES have to be one sentence.
// Two spellings of it would drift the first time either was reworded, and the
// reader would end up with a task they did not read.

func TestTheAskIsTheStepTheButtonWrites(t *testing.T) {
	t.Parallel()
	org := ids.OrganizationID{UUID: ids.NewV7()}
	out := noNextStepSuggestion(org, suggestionInputs{
		orgName: "Nordwind Logistik",
		open: pipeline{
			OpenCount: 1, OpenDigest: "d",
			Open: []openDeal{{ID: ids.NewV7(), Name: "PIM rollout Phase 2"}},
		},
	})
	if out == nil || out.Title == nil || out.Action == nil || out.Action.Task == nil {
		t.Fatalf("the rule prepared no step: %+v", out)
	}
	if *out.Title != out.Action.Task.Subject {
		t.Fatalf("the ask and the body disagree:\n  asks:   %q\n  writes: %q",
			*out.Title, out.Action.Task.Subject)
	}
	// It names WHAT to agree, not what the account has not done. "Set the next
	// step" was the finding restated as an instruction.
	if !strings.Contains(out.Action.Task.Subject, "PIM rollout Phase 2") {
		t.Errorf("subject = %q, want the open deal named in it", out.Action.Task.Subject)
	}
	// An absence has no date, and a prepared body must not invent one: a
	// deadline nobody agreed to is the reading turned into an obligation.
	if out.Action.Task.DueAt != nil {
		t.Errorf("due_at = %v, want none on a step read off an absence", out.Action.Task.DueAt)
	}
}

// A task lands in a queue where the account page is not on screen, so an
// account-level step carries the account's name. Without it the reader meets
// "Agree the next step" beside four others exactly like it.
func TestAnAccountStepCarriesTheAccountsName(t *testing.T) {
	t.Parallel()
	org := ids.OrganizationID{UUID: ids.NewV7()}
	out := noNextStepSuggestion(org, suggestionInputs{
		orgName: "Nordwind Logistik",
		open: pipeline{
			OpenCount: 2, OpenDigest: "d",
			Open: []openDeal{
				{ID: ids.NewV7(), Name: "PIM rollout Phase 2"},
				{ID: ids.NewV7(), Name: "Data quality add-on"},
			},
		},
	})
	if out == nil || out.Action == nil || out.Action.Task == nil {
		t.Fatalf("the rule prepared no step: %+v", out)
	}
	if !strings.Contains(out.Action.Task.Subject, "Nordwind Logistik") {
		t.Errorf("subject = %q, want the account named in it", out.Action.Task.Subject)
	}
	// A record with no name still gets a step rather than one addressed to
	// nobody: the finding is true whatever the row is called.
	nameless := noNextStepSuggestion(org, suggestionInputs{
		open: pipeline{OpenCount: 2, OpenDigest: "d"},
	})
	if nameless == nil || nameless.Action == nil || nameless.Action.Task == nil {
		t.Fatalf("the rule went silent on an account with no name: %+v", nameless)
	}
	if nameless.Action.Task.Subject == "" {
		t.Error("subject is empty — the step names nothing at all")
	}
}

// Which record the step hangs on is a decision the rule makes from the
// pipeline, and the same one the reason sentence makes: name the deal where
// there is exactly one, and the account where naming one would be a guess. A
// task filed against the wrong deal is worse than one filed against the
// account, which is where a rep would have put it themselves.
func TestTheStepHangsOnTheDealOnlyWhenThereIsOne(t *testing.T) {
	t.Parallel()
	org := ids.OrganizationID{UUID: ids.NewV7()}
	dealID := ids.NewV7()

	sole := noNextStepSuggestion(org, suggestionInputs{open: pipeline{
		OpenCount: 1, OpenDigest: "d",
		Open: []openDeal{{ID: dealID, Name: "PIM rollout Phase 2"}},
	}})
	if sole == nil || sole.Action == nil || sole.Action.Task == nil {
		t.Fatalf("the rule prepared no step: %+v", sole)
	}
	if kind, id := soleLink(t, *sole.Action.Task); kind != "deal" || ids.UUID(id) != dealID {
		t.Errorf("the step links %s %v, want the one open deal %v", kind, id, dealID)
	}
	// And the action names the same deal, so a client reading `deal_id` and one
	// reading the body's links cannot land on different records.
	if sole.Action.DealId == nil || ids.UUID(*sole.Action.DealId) != dealID {
		t.Errorf("action.deal_id = %v, want the deal the body links", sole.Action.DealId)
	}

	several := noNextStepSuggestion(org, suggestionInputs{open: pipeline{
		OpenCount: 2, OpenDigest: "d",
		Open: []openDeal{{ID: dealID, Name: "PIM rollout Phase 2"}, {ID: ids.NewV7(), Name: "Add-on"}},
	}})
	if several == nil || several.Action == nil || several.Action.Task == nil {
		t.Fatalf("the rule prepared no step: %+v", several)
	}
	if kind, id := soleLink(t, *several.Action.Task); kind != "organization" || ids.UUID(id) != org.UUID {
		t.Errorf("the step links %s %v, want the account %v", kind, id, org.UUID)
	}
	if several.Action.DealId != nil {
		t.Errorf("action.deal_id = %v, want none where naming one would be a guess", several.Action.DealId)
	}

	// The count can outrun the names — a deal that did not survive the read.
	// The step is still the account's, because there is no deal in hand to
	// hang it on.
	short := noNextStepSuggestion(org, suggestionInputs{open: pipeline{
		OpenCount: 1, OpenDigest: "d",
	}})
	if short == nil || short.Action == nil || short.Action.Task == nil {
		t.Fatalf("the rule prepared no step: %+v", short)
	}
	if kind, _ := soleLink(t, *short.Action.Task); kind != "organization" {
		t.Errorf("the step links %s, want the account when no deal survived the read", kind)
	}
}

// soleLink reads the one record a prepared step hangs on, failing loudly on any
// other shape: a body with no link appears on no timeline, and one with two
// would put the same step on two records.
func soleLink(
	t *testing.T, body crmcontracts.CreateTaskRequest,
) (crmcontracts.CreateTaskRequestLinksEntityType, ids.UUID) {
	t.Helper()
	if body.Links == nil || len(*body.Links) != 1 {
		t.Fatalf("links = %v, want exactly one record", body.Links)
	}
	link := (*body.Links)[0]
	return link.EntityType, ids.UUID(link.EntityId)
}
