// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// approvalsAdapter maps the tool surface's staging/redemption dependency
// onto the approvals module.
type approvalsAdapter struct{ svc *approvals.Service }

// StageCall forwards a refused 🟡 tool call to the approvals engine. It passes
// NO target_version: the engine resolves the pin itself inside the staging
// transaction, for every target type that has a version column to read. This
// adapter used to nil a caller-supplied pin for the types redemption could
// not re-verify — correct as far as it went, but it also meant the pin was
// whatever the caller happened to offer for the types it COULD, which on the
// REST path was an optional request header.
func (a approvalsAdapter) StageCall(ctx context.Context, in agents.StageRequest) (ids.ApprovalID, bool, error) {
	return a.svc.StageAgentCall(ctx, approvals.StageInput{
		Kind:           in.Tool,
		ProposedChange: in.ProposedChange,
		DiffHash:       in.DiffHash,
		TargetType:     in.TargetType,
		TargetID:       in.TargetID,
		Summary:        in.Summary,
	})
}

// StageVolumeRelease puts a §2.4 step-up in front of the human who lent the
// calling passport.
//
// It stages through the DECLINED-AWARE path, unlike Stage above, and both
// differences that drives are deliberate. JoinPending + Identity means ONE
// question per counter per window: an agent looping on a refusal re-asks
// nothing, where the ordinary path would fill an inbox with copies of one
// question and leave the rest to be dismissed after the first was answered. And
// a human's NO is remembered — a rejected step-up is not re-offered on the next
// call, which is the difference between a control and a nag.
//
// It carries NO target, because a step-up is about a credential's volume rather
// than about a record. That shape is what makes it decidable by the lender alone
// (approvals' selfOnlyKinds over a target-less staging), and it is why the
// identity carries the discrimination a diff hash carries for every other kind:
// there is no diff.
func (a approvalsAdapter) StageVolumeRelease(ctx context.Context, in agents.VolumeReleaseRequest) (ids.ApprovalID, bool, error) {
	payload, err := json.Marshal(in.Proposal)
	if err != nil {
		return ids.ApprovalID{}, false, fmt.Errorf("compose: encoding a step-up proposal: %w", err)
	}
	identity, err := in.Proposal.Identity()
	if err != nil {
		return ids.ApprovalID{}, false, err
	}
	_, diffHash, err := diffhash.Object(map[string]any{"volume_release": string(identity)})
	if err != nil {
		return ids.ApprovalID{}, false, fmt.Errorf("compose: hashing a step-up proposal: %w", err)
	}
	return a.svc.StageUnlessDeclined(ctx, approvals.StageInput{
		Kind:           approvals.KindVolumeRelease,
		ProposedChange: payload,
		DiffHash:       diffHash,
		Summary:        in.Summary,
		JoinPending:    true,
		Identity:       identity,
	})
}

func (a approvalsAdapter) Redeem(ctx context.Context, approvalID ids.ApprovalID, tool, diffHash string) (int64, bool, error) {
	return a.svc.Redeem(ctx, approvalID, tool, diffHash)
}

// State answers a polling MCP task whether a human has decided yet. The
// module's effective status is passed through rather than re-derived: a pending
// row past its window is expired on every other surface, and a poll told
// "pending" about a dead proposal would wait forever.
func (a approvalsAdapter) State(ctx context.Context, approvalID ids.ApprovalID) (agents.ApprovalState, error) {
	state, err := a.svc.TaskState(ctx, approvalID)
	if err != nil {
		return agents.ApprovalState{}, err
	}
	decided, ok := approvalDecisions[state.Status]
	if !ok {
		// A status this adapter has no word for must not be guessed at: the
		// safe-looking guess is "pending", and it would leave a released
		// approval unperformed forever.
		return agents.ApprovalState{}, fmt.Errorf("compose: unknown approval status %q", state.Status)
	}
	return agents.ApprovalState{Decided: decided, ExpiresAt: state.ExpiresAt, Consumed: state.Consumed}, nil
}

// approvalDecisions maps the approvals module's status vocabulary onto the tool
// surface's. They are the same four words today, and the map exists so they are
// not required to STAY the same by coincidence — a rename on either side lands
// on the unknown-status refusal above rather than silently reading as pending.
var approvalDecisions = map[string]agents.ApprovalDecision{
	approvals.StatusPending:  agents.ApprovalPending,
	approvals.StatusApproved: agents.ApprovalApproved,
	approvals.StatusRejected: agents.ApprovalRejected,
	approvals.StatusExpired:  agents.ApprovalExpired,
}

// ProposedChange answers what a redemption would perform — the human's edit
// where there was one. See agents.Approvals for why an executor must read this
// rather than replay what it staged.
func (a approvalsAdapter) ProposedChange(ctx context.Context, approvalID ids.ApprovalID) (json.RawMessage, error) {
	return a.svc.ProposedChange(ctx, approvalID)
}

// Withdraw retracts the proposal behind a cancelled task, so no decision is
// left in a person's inbox that could no longer take effect. It reports whether
// there was still an offer to take: a proposal a human already decided is
// untouched, and a task that claimed otherwise would say the decision was gone
// while it sat live in the inbox.
func (a approvalsAdapter) Withdraw(ctx context.Context, approvalID ids.ApprovalID) (bool, error) {
	return a.svc.Withdraw(ctx, approvalID, "the agent cancelled the task waiting on this decision")
}
