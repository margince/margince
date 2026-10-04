// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A reviewer corrects a promise's wording and its day, and nothing else: the
// locator is the memory that stops a dismissed promise coming back, and the
// party and quote are what the reviewer agreed to.
func TestACommitmentEditReachesOnlyTheWordingAndTheDay(t *testing.T) {
	staged := CommitmentTaskProposal{
		SourceActivityID: ids.NewV7(), Summary: "Send the revised pricing", Party: "Priya",
		DueDate: "2026-09-08", Quote: "I'll send the revised pricing over by Friday.",
		Locator: "abc", Body: "Priya committed to this.",
	}
	for _, c := range []struct {
		name   string
		edit   func(p *CommitmentTaskProposal)
		refuse bool
	}{
		{"the wording", func(p *CommitmentTaskProposal) { p.Summary = "Send pricing v2" }, false},
		{"the day", func(p *CommitmentTaskProposal) { p.DueDate = "2026-09-09" }, false},
		{"clearing the day", func(p *CommitmentTaskProposal) { p.DueDate = "" }, false},
		{"clearing the wording", func(p *CommitmentTaskProposal) { p.Summary = "  " }, true},
		{"a day that is not a date", func(p *CommitmentTaskProposal) { p.DueDate = "Friday" }, true},
		{"the locator", func(p *CommitmentTaskProposal) { p.Locator = "xyz" }, true},
		{"the party", func(p *CommitmentTaskProposal) { p.Party = "Dana" }, true},
		{"the quote", func(p *CommitmentTaskProposal) { p.Quote = "something else" }, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			edited := staged
			c.edit(&edited)
			err := commitmentTaskPrecheck()(context.Background(), mustJSON(t, staged), mustJSON(t, edited))
			var invalid *approvals.InvalidEditError
			if c.refuse && !errors.As(err, &invalid) {
				t.Errorf("editing %s: err = %v, want an invalid edit", c.name, err)
			}
			if !c.refuse && err != nil {
				t.Errorf("editing %s was refused: %v", c.name, err)
			}
		})
	}
}

func mustJSON(t *testing.T, v CommitmentTaskProposal) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return raw
}

// A payload that will not decode is refused, not read as an empty edit.
func TestACommitmentEditThatIsNotAProposalIsRefused(t *testing.T) {
	staged := mustJSON(t, CommitmentTaskProposal{Summary: "Send pricing", Locator: "abc"})
	var invalid *approvals.InvalidEditError
	if err := commitmentTaskPrecheck()(context.Background(), staged, json.RawMessage(`{"summary":`)); !errors.As(err, &invalid) {
		t.Errorf("an edit that is not JSON: err = %v, want an invalid edit", err)
	}
	if err := commitmentTaskPrecheck()(context.Background(), json.RawMessage(`[`), staged); err == nil {
		t.Error("a staged payload that is not JSON was compared as if it were")
	}
	if err := commitmentTaskPrecheck()(context.Background(), staged, nil); err != nil {
		t.Errorf("accepting with no edit: %v", err)
	}
}

// Accepting a commitment refuses before it spends the approval: a payload it
// cannot read, and a decision nobody made.
func TestAcceptingACommitmentRefusesWhatItCannotWrite(t *testing.T) {
	effect := commitmentTaskEffect(nil, nil, nil)
	if err := effect(context.Background(), ids.From[ids.ApprovalKind](ids.NewV7()), json.RawMessage(`{`), "h"); err == nil {
		t.Error("an unreadable proposal was accepted")
	}
	payload := mustJSON(t, CommitmentTaskProposal{Summary: "Send pricing", Locator: "abc"})
	if err := effect(context.Background(), ids.From[ids.ApprovalKind](ids.NewV7()), payload, "h"); err == nil {
		t.Error("a proposal was accepted with nobody deciding it")
	}
	// A promise in private mail lands on its owner or nobody.
	owner, other := ids.NewV7(), ids.NewV7()
	private := mustJSON(t, CommitmentTaskProposal{Summary: "Send pricing", Locator: "abc", PrivateTo: &owner})
	decider := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + other.String(), UserID: other,
	})
	if err := effect(decider, ids.From[ids.ApprovalKind](ids.NewV7()), private, "h"); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a colleague accepting a promise from another member's private mail: err = %v, want permission denied", err)
	}
}

// The extractor's name is put on the product's own pass, and nothing else is
// turned into one: a human stays human, and no principal stays none.
func TestTheExtractorNameNeverWidensACaller(t *testing.T) {
	if _, ok := principal.Actor(extractorContext(context.Background(), "agent:reader")); ok {
		t.Error("a context with no principal came back with one")
	}
	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:x",
	})
	if actor, _ := principal.Actor(extractorContext(human, "agent:reader")); actor.Type != principal.PrincipalHuman || actor.ID != "human:x" {
		t.Errorf("a human came back as %s %q", actor.Type, actor.ID)
	}
	pass := principal.SystemActing(context.Background(), "agent:pass")
	if actor, _ := principal.Actor(extractorContext(pass, "agent:reader")); actor.Type != principal.PrincipalSystem || actor.ID != "agent:reader" {
		t.Errorf("the product's pass is filed as %s %q, want the system under the reader's name", actor.Type, actor.ID)
	}
}
