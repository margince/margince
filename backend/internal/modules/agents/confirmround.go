// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// The registry's two rounds of a call confirmed in the conversation
// (confirmcode.go). The preview stages nothing. The spend releases the call as
// a redeemed approval would.

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// RecordPinner answers the pins a staging of a call would take, without staging
// it. The approvals seam implements it. A seam that does not leaves the code
// unpinned, and the diff-hash binding still holds.
type RecordPinner interface {
	PinRecords(ctx context.Context, in StageRequest) (auth.ReleasedRecordPins, error)
}

// previewInConversation answers round 1: what the call would do, and the code its
// user's yes sends back. Nothing is staged and nothing runs. A tool that cannot
// describe its own target has nothing to preview, so its refusal stands.
//
// The code holds no more than the caller could release from the inbox itself.
// A call it could not release, one that cannot be undone, is staged for a
// colleague to decide there.
func (r *Registry) previewInConversation(ctx context.Context, t mcp.Tool, spec mcp.ToolSpec, args json.RawMessage, diffHash string, refusal error) error {
	stageable, ok := t.(stageableTool)
	if !ok {
		return refusal
	}
	info, err := stageable.StageInfo(ctx, args)
	if err != nil {
		return err
	}
	if !r.approvals.ReleasableByCaller(ctx, releaseRequest(spec.Name, diffHash, args, info.TargetType, info.TargetID)) {
		return r.stageRefusedCall(ctx, t, spec.Name, args, diffHash, refusal)
	}
	// A preview reads, so it counts against the call ceiling like any read: a
	// caller cannot preview without bound.
	if err := r.chargeCall(ctx, spec, nothingHasHappenedYet); err != nil {
		return err
	}
	pins, err := r.previewPins(ctx, spec.Name, diffHash, args, info)
	if err != nil {
		return err
	}
	c, _ := conversationConfirmFrom(ctx)
	code, expires, err := c.mint(ctx, wireConfirmCode{
		Tool: spec.Name, Args: diffHash, TargetType: info.TargetType, TargetID: info.TargetID, Pins: pinsOnWire(pins),
	})
	if err != nil {
		return err
	}
	return &ConfirmInConversationError{Tool: spec.Name, Summary: info.Summary, Code: code, ExpiresAt: expires}
}

// previewPins are the pins the code carries: what a staging of this call would
// pin, asked of the approvals engine. Never the tool's own read, which a staging
// discards too.
func (r *Registry) previewPins(ctx context.Context, tool, diffHash string, args json.RawMessage, info StageInfo) (auth.ReleasedRecordPins, error) {
	pinner, ok := r.approvals.(RecordPinner)
	if !ok {
		return auth.ReleasedRecordPins{}, nil
	}
	in := releaseRequest(tool, diffHash, args, info.TargetType, info.TargetID)
	in.CoTargetType, in.CoTargetID = info.CoTargetType, info.CoTargetID
	return pinner.PinRecords(ctx, in)
}

func releaseRequest(tool, diffHash string, args json.RawMessage, targetType string, targetID ids.UUID) StageRequest {
	return StageRequest{Tool: tool, ProposedChange: args, DiffHash: diffHash, TargetType: targetType, TargetID: targetID}
}

// spendConfirmCode is round 2's release: the code is checked against this very
// call and spent once. The run is charged here, as a redeemed approval's is,
// because the gate refused the call and charged nothing.
func (r *Registry) spendConfirmCode(ctx context.Context, spec mcp.ToolSpec, res reserved) (context.Context, error) {
	c, armed := conversationConfirmFrom(ctx)
	if !armed {
		return ctx, errConfirmCodeNotHere
	}
	marked, claim, err := c.spend(ctx, spec.Name, res.DiffHash, res.ConfirmCode)
	if err != nil {
		return ctx, err
	}
	// Asked again at the spend: a project held since the preview makes the
	// filing one the caller can no longer release.
	if r.approvals == nil || !r.approvals.ReleasableByCaller(ctx, releaseRequest(spec.Name, res.DiffHash, res.Args, claim.TargetType, claim.TargetID)) {
		return ctx, errConfirmCodeNoLonger
	}
	marked = storekit.WithConfirmingCode(marked, claim.Once)
	r.ChargeRedeemedCall(marked, spec)
	return marked, nil
}

// redeemPresented spends the release a retry presents, an approval or a confirm
// code, and answers the context marked released. A call presenting neither passes
// through: the gate already decided whether one was required.
//
// The ceiling is charged after the release succeeds, and absorbed, not refused.
// The release is spent by then, and refusing would burn it on a call that never
// ran.
func (r *Registry) redeemPresented(ctx context.Context, spec mcp.ToolSpec, res reserved) (context.Context, error) {
	if res.ConfirmCode != "" {
		return r.spendConfirmCode(ctx, spec, res)
	}
	if res.ApprovalID.IsZero() {
		return ctx, nil
	}
	if r.approvals == nil {
		return ctx, fmt.Errorf("this call presents an approval_id, but this surface has no approvals to redeem it against: %w", apperrors.ErrApprovalTokenInvalid)
	}
	marked, _, _, err := RedeemAndMark(ctx, r.approvals, res.ApprovalID, spec.Name, res.DiffHash)
	if err != nil {
		return ctx, err
	}
	r.ChargeRedeemedCall(marked, spec)
	return marked, nil
}
