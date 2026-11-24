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
	"log/slog"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/mcp"
)

// TargetPinner answers the version a staging would pin for a target. The
// approvals seam implements it; a seam that does not leaves the code unpinned,
// which the diff-hash binding still holds.
type TargetPinner interface {
	PinTarget(ctx context.Context, kind, targetType string, targetID ids.UUID) (int64, bool, error)
}

type confirmOnceKey struct{}

// previewInConversation answers round 1: what the call would do, and the code its
// user's yes sends back. Nothing is staged and nothing runs. A tool that cannot
// describe its own target has nothing to preview, so its refusal stands.
func (r *Registry) previewInConversation(ctx context.Context, t mcp.Tool, spec mcp.ToolSpec, args json.RawMessage, diffHash string, refusal error) error {
	stageable, ok := t.(stageableTool)
	if !ok {
		return refusal
	}
	info, err := stageable.StageInfo(ctx, args)
	if err != nil {
		return err
	}
	// A preview reads, so it counts against the call ceiling like any read: a
	// caller cannot preview without bound.
	if err := r.chargeCall(ctx, spec, nothingHasHappenedYet); err != nil {
		return err
	}
	version, err := r.previewVersion(ctx, spec.Name, info)
	if err != nil {
		return err
	}
	c, _ := conversationConfirmFrom(ctx)
	code, expires, err := c.mint(ctx, spec.Name, diffHash, previewedTarget{Type: info.TargetType, ID: info.TargetID, Version: version})
	if err != nil {
		return err
	}
	return &ConfirmInConversationError{Tool: spec.Name, Summary: info.Summary, Code: code, ExpiresAt: expires}
}

// previewVersion is the version the code pins: what the tool's own read saw, or
// else what a staging of this call would have pinned.
func (r *Registry) previewVersion(ctx context.Context, kind string, info StageInfo) (*int64, error) {
	if info.TargetVersion != nil {
		return info.TargetVersion, nil
	}
	pinner, ok := r.approvals.(TargetPinner)
	if !ok || info.TargetID.IsZero() {
		return nil, nil
	}
	version, pinned, err := pinner.PinTarget(ctx, kind, info.TargetType, info.TargetID)
	if err != nil || !pinned {
		return nil, err
	}
	return &version, nil
}

// spendConfirmCode is round 2's release: the code is checked against this very
// call and spent once. The run is charged here, as a redeemed approval's is,
// because the gate refused the call and charged nothing.
func (r *Registry) spendConfirmCode(ctx context.Context, spec mcp.ToolSpec, res reserved) (context.Context, error) {
	c, armed := conversationConfirmFrom(ctx)
	if !armed {
		return ctx, errConfirmCodeNotHere
	}
	marked, once, err := c.spend(ctx, spec.Name, res.DiffHash, res.ConfirmCode)
	if err != nil {
		return ctx, err
	}
	marked = storekit.WithConfirmingCode(context.WithValue(marked, confirmOnceKey{}, once), once)
	r.ChargeRedeemedCall(marked, spec)
	return marked, nil
}

// releaseUnusedConfirmCode gives a spent code back when the call it released
// failed before anything committed, so the user's yes still covers the retry.
func (r *Registry) releaseUnusedConfirmCode(ctx context.Context) {
	once, spent := ctx.Value(confirmOnceKey{}).(string)
	c, armed := conversationConfirmFrom(ctx)
	if !spent || !armed {
		return
	}
	if err := c.guard.Release(context.WithoutCancel(ctx), confirmCodeTyp+":"+once); err != nil {
		slog.Default().WarnContext(ctx, "mcp: a confirm code could not be released after a run that changed nothing", "err", err)
	}
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
