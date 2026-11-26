// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// The renderer joins the parts a reader has to receive in order — the outcome
// first — and drops the ones an entry legitimately has nothing to say about,
// rather than leaving the gaps as double spaces a client renders verbatim.
func TestToolCopyRendersTheWrittenPartsInOrder(t *testing.T) {
	full := toolCopy{
		Purpose: "Does the thing.",
		Limits:  "Not the other thing.",
		Instead: "Use the neighbor for that.",
		Retain:  "Keep the id.",
	}
	want := "Does the thing. Not the other thing. Use the neighbor for that. Keep the id."
	if got := full.render(); got != want {
		t.Errorf("render() = %q, want %q", got, want)
	}

	// A tool with no confusable neighbour and nothing to carry forward says so
	// by omission. The result must still be exactly its purpose: an entry that
	// rendered as "Does the thing.   " would put trailing whitespace on the
	// wire for every such tool.
	purposeOnly := toolCopy{Purpose: "Does the thing."}
	if got := purposeOnly.render(); got != "Does the thing." {
		t.Errorf("purpose-only render() = %q, want the purpose alone", got)
	}

	// Whitespace-only parts are the same claim as absent ones. Left in, they
	// would each contribute a separator to a description that says nothing more.
	blank := toolCopy{Purpose: "Does the thing.", Limits: "   ", Retain: "\t"}
	if got := blank.render(); got != "Does the thing." {
		t.Errorf("blank-part render() = %q, want the purpose alone", got)
	}
}

// The written text is what a model selects on; the governance clause is what
// happens once it has. Both must be present, and the written half comes first:
// a model reading only the opening of a thirty-tool listing must be reading the
// answer to "what is this for".
func TestDescribeForClientLeadsWithTheWrittenTextAndAppendsGovernance(t *testing.T) {
	spec := mcp.ToolSpec{
		Name:          "send_email",
		Description:   "Put a mail on the wire.",
		RequiredScope: principal.ScopeSend,
		Tier:          mcp.TierConfirmationRequired,
		// A crm.yaml operation family is developer documentation. A model
		// cannot call an endpoint, so naming one spends the description's
		// opening on something it can never act on.
		OpenAPIOp: "sendEmail",
	}
	got := DescribeForClient(spec)
	if !strings.HasPrefix(got, "Put a mail on the wire.") {
		t.Errorf("description = %q, want it to open with the written text", got)
	}
	if !strings.Contains(got, "a human approves every call before it runs") {
		t.Errorf("description = %q, want the confirm-first tier stated", got)
	}
	if !strings.Contains(got, `"send"`) {
		t.Errorf("description = %q, want the required scope stated", got)
	}
	if strings.Contains(got, "sendEmail") {
		t.Errorf("description = %q, want the crm.yaml operation left out of the model-facing text", got)
	}
}

// A scope past the defaults is named even when the call runs at once. Read and
// write are the defaults the server instructions state, so neither is repeated.
func TestDescribeForClientNamesEveryScopePastTheDefaults(t *testing.T) {
	describe := func(scope principal.Scope) string {
		return DescribeForClient(mcp.ToolSpec{
			Name: "t", Description: "Does the thing.", RequiredScope: scope, Tier: mcp.TierAutoExecute,
		})
	}
	for _, scope := range []principal.Scope{principal.ScopeRead, principal.ScopeWrite} {
		if got := describe(scope); got != "Does the thing." {
			t.Errorf("auto-execute %s reads %q, want the written text alone", scope, got)
		}
	}
	if got := describe(principal.ScopeSend); got != `Does the thing. (Requires passport scope "send".)` {
		t.Errorf("auto-execute send reads %q, want the scope named and no approval asked", got)
	}
}

// The instructions carry what the descriptions leave out: the default tier and
// scope, the ask-first rule and the retry key. A description is read beside them.
func TestTheInstructionsStateWhatEveryDescriptionLeavesOut(t *testing.T) {
	served := NewDispatcher(NewRegistry(nil, nil), bindAuthenticated, "margince-crm", "test").instructions()
	for _, want := range []string{
		"names no governance runs immediately", `"write" if it changes anything`,
		askFirstOpening, askConsequenceWrite, askConsequenceRead, askFirstGetYes, mcp.ReservedIdempotencyKeyRule,
	} {
		if !strings.Contains(served, want) {
			t.Errorf("the instructions lack %q:\n%s", want, served)
		}
	}
}

// The dynamic tier is the one a single sentence can get wrong: a deal move is
// auto-execute until the target stage closes the deal. A description that
// reported it as one or the other would be false half the time.
func TestDescribeForClientStatesEachTierDistinctly(t *testing.T) {
	describe := func(tier mcp.RiskTier) string {
		return DescribeForClient(mcp.ToolSpec{
			Name: "t", Description: "Does the thing.", RequiredScope: principal.ScopeRead, Tier: tier,
		})
	}
	// Running at once under the read scope is what every tool does unless it
	// says otherwise, so that tool's description carries nothing more.
	if got := describe(mcp.TierAutoExecute); got != "Does the thing." {
		t.Errorf("auto-execute read reads %q, want the written text alone", got)
	}
	if got := describe(mcp.TierConfirmationRequired); !strings.Contains(got, "approves every call") {
		t.Errorf("confirm-first reads %q, want every call named as needing a contact", got)
	}
	// The dynamic tier is the one a single sentence gets wrong by collapsing:
	// it must promise neither that the call runs nor that a human approves it,
	// because which one is true is decided per call from the arguments.
	dynamic := describe(mcp.TierDynamic)
	if !strings.Contains(dynamic, "decided per call") {
		t.Errorf("dynamic reads %q, want it to say the tier is resolved per call", dynamic)
	}
	if strings.Contains(dynamic, "approves every call") {
		t.Errorf("dynamic reads %q, so an open→open move looks like it needs approval", dynamic)
	}
	// It must not name a deal: nothing holds TierDynamic to the deal moves that
	// use it today, and a clause naming them would be false for the next one.
	if strings.Contains(dynamic, "deal") {
		t.Errorf("dynamic reads %q, want the clause to describe the tier rather than today's only users", dynamic)
	}
}

// A read tool that asks for the user's yes first must say so. The read scope
// otherwise carries nothing more, which hid the ask. A read that surfaces AI
// call diagnostics (get_ai_call) is then called unprompted by a client never
// told to ask.
func TestDescribeForClientSurfacesAnAskAReadRequires(t *testing.T) {
	got := DescribeForClient(mcp.ToolSpec{
		Name: "get_ai_call", Description: "Read one AI call.",
		RequiredScope: principal.ScopeRead, Tier: mcp.TierAutoExecute, AskUserFirst: true,
	})
	if got != "Read one AI call. (Ask the user first.)" {
		t.Errorf("a read tool that asks first reads %q, want the ask-first mark after the written text", got)
	}
}

// A tool marked ask-first that runs with no human approval tells the client to get
// the user's yes first. A call a human approves already has that yes.
func TestDescribeForClientAsksTheUserFirstOnlyWhereNoHumanApproves(t *testing.T) {
	describe := func(tier mcp.RiskTier, askFirst bool) string {
		return DescribeForClient(mcp.ToolSpec{
			Name: "t", Description: "Retires the thing.", RequiredScope: principal.ScopeWrite,
			Tier: tier, AskUserFirst: askFirst,
		})
	}
	if got := describe(mcp.TierAutoExecute, true); got != "Retires the thing. (Ask the user first.)" {
		t.Errorf("auto-execute archive reads %q, want the client told to ask the user first", got)
	}
	if got := describe(mcp.TierConfirmationRequired, true); strings.Contains(got, askFirstOpening) {
		t.Errorf("confirm-first archive reads %q, want no second confirmation asked", got)
	}
	if got := describe(mcp.TierAutoExecute, false); strings.Contains(got, askFirstOpening) {
		t.Errorf("an ordinary write reads %q, want no confirmation asked", got)
	}
}

// A call the user already logged is found on the timeline before it is logged
// again, as a contact is found before it is created.
func TestLoggingAnActivityReadsTheTimelineFirst(t *testing.T) {
	served := changeActivities{}.Spec().Description
	for _, want := range []string{"Before log", "read_context about catch_up", "never logged a second time"} {
		if !strings.Contains(served, want) {
			t.Errorf("change_activities copy lacks %q:\n%s", want, served)
		}
	}
}
