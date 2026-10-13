// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

// Whose work the promise card says is owed.

import (
	"context"
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// An agent claims nobody's work as its own, including its granting human's.
//
// A passport carries that human's UserID, so comparing ids alone answers about
// them.
//
// The card read "You owe them" at a credential that owes nothing. The marker
// keyed as ":ours" put the agent in the human's dismissal namespace.
func TestAnAgentHoldsNobodysPromise(t *testing.T) {
	t.Parallel()
	holder := ids.NewV7()
	assignee := openapi_types.UUID(holder)
	task := crmcontracts.Activity{AssigneeId: &assignee}

	human := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:holder", UserID: holder,
	})
	if heldByAnother(human, task) {
		t.Error("the task's own assignee is told a colleague holds it")
	}

	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:passport", UserID: holder,
	})
	if !heldByAnother(agent, task) {
		t.Error("an agent carrying its human's id claimed that human's task as its own")
	}
	if got := heldMarker(agent, task); got != ":theirs" {
		t.Errorf("the agent's dismissal marker is %q, which shares the human's namespace", got)
	}

	// A call carrying nobody holds nothing either. This arm is the human gate
	// refusing, because there is no actor to admit.
	if !heldByAnother(context.Background(), task) {
		t.Error("a call with no principal at all claimed the task as its own")
	}

	// The id check is still reachable behind that gate. RequireHuman admits a
	// human principal and asks nothing about its user id, so a human carrying
	// none reaches the comparison.
	anonymous := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:no-user-id",
	})
	if !heldByAnother(anonymous, task) {
		t.Error("a human principal with no user id claimed the task as its own")
	}
}

// An agent's baseline answer needs no query, which is how this reads it with
// no transaction at all.
//
// The guard runs before the user lookup and before the statement, so an agent
// is answered from the context. A test that reached a database would prove the
// same thing more slowly and would not prove the ordering.
func TestAnAgentsBaselineIsAnsweredWithoutAQuery(t *testing.T) {
	t.Parallel()
	svc := &Service{}
	agent := principal.WithActor(context.Background(), principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:passport", UserID: ids.NewV7(),
	})

	at, visited, err := svc.baselineFor(agent, nil, ids.From[ids.ContactKind](ids.NewV7()))
	if err != nil {
		t.Fatalf("an agent's baseline answered %v", err)
	}
	if visited {
		t.Error("an agent was reported as having visited, which is its human's mark")
	}
	if !at.IsZero() {
		t.Errorf("an agent carries a baseline of %v, which can only be its human's", at)
	}
}

// A call carrying no principal is an error, not an empty baseline.
//
// The two arms are worth telling apart. An agent has no mark of its own and
// that is a fact about the caller, so it answers nil. A call with nobody
// behind it is a wiring fault, and the human gate reports it as one.
func TestABaselineWithNoPrincipalIsRefused(t *testing.T) {
	t.Parallel()
	svc := &Service{}

	_, visited, err := svc.baselineFor(context.Background(), nil, ids.From[ids.ContactKind](ids.NewV7()))
	if err == nil {
		t.Fatal("a call with no principal read a baseline instead of failing")
	}
	if visited {
		t.Error("a refused call reported a visit")
	}
}
