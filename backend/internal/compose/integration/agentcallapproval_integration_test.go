// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One agent call, one authority object — however many times the agent asks.
//
// The reported loop, against real Postgres: an `enrich` call was refused 🟡,
// staged, approved by a human, and then staged AGAIN three more times, so one
// organization carried four approvals at the same version with the same diff
// hash. A human answered all four and not one was ever spent. Two things made
// that possible and both are proven here — the gate minted a fresh approval per
// attempt instead of recognizing the one already on the table, and a retry that
// landed before the human clicked was told its token was INVALID, which leaves
// an agent nothing to do but ask again.
//
// It drives send_message rather than enrich because the invariant belongs to the
// gate, not the verb: both refusals go through agents.Registry.stageRefusedCall
// into approvals.StageAgentCall, and send_message needs no network to reach it.

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/compose/integration/apptest"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/ports/workflow"
)

func TestOneRefusedAgentCallCollectsExactlyOneApprovalHoweverOftenItIsRetried(t *testing.T) {
	// Driven through `enrich` rather than a send: what this proves is the
	// APPROVAL mechanism — one row per refused call, single-use redemption —
	// and that needs a verb that still stages. A passport no longer needs a
	// second confirmation from the person who granted it, so the sends do not.
	c := setupChannelSend(t)
	invoke := c.enrichInvoker(t, c.mintPassport(t, []string{"read", "enrich"}))
	org := c.enrichTarget(t)

	args := enrichArgs(org)
	retry := func(approvalID string) string { return enrichRetry(org, approvalID) }

	first := c.stagedApproval(t, invoke, args)
	if first.AlreadyApproved {
		t.Fatal("the first refusal claims a human has already approved it")
	}

	// The same call again, undecided: the agent is handed the approval already
	// on the table, not a second one for the human to answer.
	second := c.stagedApproval(t, invoke, args)
	if second.ApprovalID != first.ApprovalID {
		t.Fatalf("re-asking staged approval %s, want the live one (%s)", second.ApprovalID, first.ApprovalID)
	}
	if second.AlreadyApproved {
		t.Fatal("an undecided approval was reported as already approved")
	}
	c.assertApprovalCount(t, 1, "two identical refused calls")

	// A retry that lands before the human clicks. This is the answer the loop
	// turned on: "requires approval", naming the id to keep, and NOT
	// "approval token invalid" — which an agent can only read as "that id is
	// dead, ask again".
	_, earlyErr := invoke(retry(first.ApprovalID.String()))
	if !errors.Is(earlyErr, apperrors.ErrRequiresApproval) {
		t.Fatalf("retry before the decision → %v, want ErrRequiresApproval", earlyErr)
	}
	if errors.Is(earlyErr, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("retry before the decision → %v, must not read as an invalid token", earlyErr)
	}
	c.assertApprovalCount(t, 1, "an early retry")

	if status := c.Call(t, "POST", "/v1/approvals/"+first.ApprovalID.String()+"/approve", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}

	// After the decision, an identical call carrying no approval_id — the shape a
	// retry takes when the agent has discarded the id it was given — is pointed at
	// the decision that already exists.
	released := c.stagedApproval(t, invoke, args)
	if released.ApprovalID != first.ApprovalID {
		t.Fatalf("after approval the gate answered %s, want the approved one (%s)", released.ApprovalID, first.ApprovalID)
	}
	if !released.AlreadyApproved {
		t.Fatal("the gate did not tell the agent its call is already approved")
	}
	c.assertApprovalCount(t, 1, "an identical call after the approval")

	// The approved retry REACHES Handle, which is what this asserts. It then
	// fails inside enrich for a reason that has nothing to do with the
	// approval — no model path is configured in this deployment — and that
	// refusal is the proof it got through: an unredeemed call never arrives at
	// the tool at all, it is turned away by the gate with ErrRequiresApproval.
	_, redeemed := invoke(retry(first.ApprovalID.String()))
	if errors.Is(redeemed, apperrors.ErrRequiresApproval) || errors.Is(redeemed, apperrors.ErrApprovalTokenInvalid) {
		t.Fatalf("approved retry → %v, want it past the gate and into the tool", redeemed)
	}

	// A SPENT decision is not standing authority. The identical call now needs a
	// new question, and gets exactly one.
	afterSpending := c.stagedApproval(t, invoke, args)
	if afterSpending.ApprovalID == first.ApprovalID {
		t.Fatal("the consumed approval was handed back; a decision is single-use")
	}
	if afterSpending.AlreadyApproved {
		t.Fatal("a call whose approval was spent was told it is already approved")
	}
	c.assertApprovalCount(t, 2, "a call made after its approval was spent")
}

// stagedApproval invokes the tool and insists the answer is a 🟡 staging.
func (c *channelSendEnv) stagedApproval(t *testing.T, invoke func(string) (string, error), args string) *workflow.StagedApprovalError {
	t.Helper()
	_, err := invoke(args)
	var staged *workflow.StagedApprovalError
	if !errors.As(err, &staged) {
		t.Fatalf("call → %v, want a StagedApprovalError", err)
	}
	if staged.ApprovalID.IsZero() {
		t.Fatal("StagedApprovalError carries a zero approval id")
	}
	return staged
}

// assertApprovalCount counts the send_message approvals the workspace holds.
// The number IS the bug: four rows for one act is what a human saw in the inbox.
func (c *channelSendEnv) assertApprovalCount(t *testing.T, want int, after string) {
	t.Helper()
	c.assertApprovalCountOf(t, "enrich", want, after)
}

// assertApprovalCountOf counts the rows staged for ONE verb. Named per verb
// rather than counting the whole table: a test that stages an enrich must not
// pass because some other call left a row behind.
func (c *channelSendEnv) assertApprovalCountOf(t *testing.T, kind string, want int, after string) {
	t.Helper()
	var got int
	if err := apptest.InWorkspace(c.AppEnv, t, c.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE kind = $1`, kind).Scan(&got)
	}); err != nil {
		t.Fatalf("counting the staged approvals: %v", err)
	}
	if got != want {
		t.Fatalf("%d %s approvals after %s, want %d", got, kind, after, want)
	}
}

// The approval handed back has to be one the caller could actually SPEND, or the
// gate has traded a duplicate inbox card for a dead end: the agent is told to
// present an id, presents it, and is refused for a reason it cannot fix.
//
// A target that moved is the case that bit staging hardest. Four approved
// enrich tokens were left pinned to organization v2 after the record reached v3,
// so every one of them would now fail the redemption's own skew check — pointing
// a retry at any of them would strand the call permanently.
func TestAnApprovedCallWhoseTargetMovedIsStagedAfreshRatherThanHandedBackDead(t *testing.T) {
	c := setupChannelSend(t)
	invoke := c.enrichInvoker(t, c.mintPassport(t, []string{"read", "enrich"}))
	org := c.enrichTarget(t)
	args := enrichArgs(org)

	staged := c.stagedApproval(t, invoke, args)
	if status := c.Call(t, "POST", "/v1/approvals/"+staged.ApprovalID.String()+"/approve", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}
	// A human edits the record the approval is pinned to, which is ordinary work
	// on the very company the enrich would rewrite.
	if status := c.Call(t, "PATCH", "/v1/organizations/"+org,
		apptest.AnyMap{"industry": "logistics"}, nil, nil); status != http.StatusOK {
		t.Fatalf("human edit of the target → %d", status)
	}

	afterMove := c.stagedApproval(t, invoke, args)
	if afterMove.AlreadyApproved {
		t.Fatal("the gate offered an approval pinned to a version the target has left")
	}
	if afterMove.ApprovalID == staged.ApprovalID {
		t.Fatal("the gate handed back the stale approval instead of asking the question again")
	}
	c.assertApprovalCount(t, 2, "the target moved past the approved pin")
}

// A decision is bound to the passport it was staged by (ADR-0036 — the row IS
// the authority object), so recognizing "this call already has an approval" must
// not reach across credentials. Handing passport B the approval a human granted
// to passport A would let a narrower or revoked credential inherit authority it
// was never given.
//
// This is the end-to-end half and it deliberately proves less than it looks like
// it does: BOTH callers present a passport here, and the redemption refuses a
// cross-passport token on its own, so this would still pass over a probe that
// scoped nothing. The case that needs the probe's own predicate is a caller with
// NO passport, whom the redemption admits — that one is held in
// approvals.TestAnAgentCallIsOnlyOfferedApprovalsItsOwnCredentialHolds, where the
// principal can be built without one.
func TestAnApprovalStagedByOnePassportIsNeverOfferedToAnother(t *testing.T) {
	c := setupChannelSend(t)
	org := c.enrichTarget(t)
	args := enrichArgs(org)

	first := c.enrichInvoker(t, c.mintPassport(t, []string{"read", "enrich"}))
	mine := c.stagedApproval(t, first, args)
	if status := c.Call(t, "POST", "/v1/approvals/"+mine.ApprovalID.String()+"/approve", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}

	second := c.enrichInvoker(t, c.mintPassport(t, []string{"read", "enrich"}))
	theirs := c.stagedApproval(t, second, args)
	if theirs.AlreadyApproved {
		t.Fatal("a second passport was told the call it is making is already approved")
	}
	if theirs.ApprovalID == mine.ApprovalID {
		t.Fatal("a second passport was handed the approval granted to the first")
	}
	c.assertApprovalCount(t, 2, "a second passport asked the same question")
}

// The REST door's own loop. It hashes a call through its own spelling
// (canonicalRESTCall, not the tool surface's diffhash) and writes its own
// refusal prose, so "the gate collects one approval per call" is a claim about
// both doors or about neither — and the recurring reviewer catch here is a fix
// that landed on the case under review and missed the sibling copy.
func TestTheRESTDoorAlsoCollectsExactlyOneApprovalPerIdenticalCall(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var person struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/people", apptest.AnyMap{"full_name": "Greta Human"}, nil, &person); status != http.StatusCreated {
		t.Fatalf("human create → %d", status)
	}
	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", apptest.AnyMap{
		"label": "duplicate-approval agent", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}
	overwrite := apptest.AnyMap{"full_name": "Greta Machine"}

	stage := func(what string) (id, detail string) {
		t.Helper()
		var problem struct {
			Code   string `json:"code"`
			Detail string `json:"detail"`
		}
		status := e.Call(t, "PATCH", "/v1/people/"+person.ID, overwrite, bearer, &problem)
		if status != http.StatusForbidden || problem.Code != "approval_required" {
			t.Fatalf("%s → %d %q, want 403 approval_required", what, status, problem.Code)
		}
		return ExtractStagedApprovalID(t, problem.Detail), problem.Detail
	}

	first, _ := stage("the first refused overwrite")
	second, secondDetail := stage("the identical overwrite, still undecided")
	if second != first {
		t.Fatalf("re-sending staged approval %s, want the live one (%s)", second, first)
	}
	if !strings.Contains(secondDetail, "staged as approval") {
		t.Fatalf("undecided refusal %q does not tell the agent to wait for a human", secondDetail)
	}
	assertPersonApprovalCount(t, e, 1, "two identical refused PATCHes")

	if status := e.Call(t, "POST", "/v1/approvals/"+first+"/approve", apptest.AnyMap{}, nil, nil); status != http.StatusOK {
		t.Fatalf("human approve → %d", status)
	}

	// The identical request WITHOUT the token, after the decision: pointed at
	// the approval it already has, and told so in words it can act on.
	released, releasedDetail := stage("the identical overwrite after approval")
	if released != first {
		t.Fatalf("after approval the door answered %s, want the approved one (%s)", released, first)
	}
	if !strings.Contains(releasedDetail, "already approved this exact request") {
		t.Fatalf("refusal %q does not tell the agent its request is already approved", releasedDetail)
	}
	assertPersonApprovalCount(t, e, 1, "an identical PATCH after the approval")

	withToken := map[string]string{"Authorization": "Bearer " + minted.Token, "X-Approval-Token": first}
	if status := e.Call(t, "PATCH", "/v1/people/"+person.ID, overwrite, withToken, nil); status != http.StatusOK {
		t.Fatalf("approved retry → %d, want the patch to execute", status)
	}
	var current struct {
		FullName string `json:"full_name"`
	}
	if status := e.Call(t, "GET", "/v1/people/"+person.ID, nil, bearer, &current); status != http.StatusOK || current.FullName != "Greta Machine" {
		t.Fatalf("approved overwrite did not land: %d %q", status, current.FullName)
	}
}

// assertPersonApprovalCount counts the update_record approvals in the workspace.
func assertPersonApprovalCount(t *testing.T, e *apptest.AppEnv, want int, after string) {
	t.Helper()
	var got int
	if err := apptest.InWorkspace(e, t, e.Slug, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM approval WHERE kind = 'update_record'`).Scan(&got)
	}); err != nil {
		t.Fatalf("counting the staged approvals: %v", err)
	}
	if got != want {
		t.Fatalf("%d update_record approvals after %s, want %d", got, after, want)
	}
}
