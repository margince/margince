// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Test doubles the tool specs share: an approvals engine that records what it
// was asked, and a native record fixture.

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// recordingApprovals captures staging and redemption for assertions.
type recordingApprovals struct {
	staged    []StageRequest
	steppedUp []VolumeReleaseRequest
	redeemed  []string // "id tool hash"
	redeemErr error
	// stepUpDeclined replays a human who has already said no. Nothing is staged.
	// The volume budget refusal stands on its own and is not an error about
	// staging.
	stepUpDeclined bool
	stepUpErr      error
	// alreadyApproved replays the engine recognizing a decision this caller
	// already holds. The call is not staged again. The refusal tells the agent to
	// spend the approval and not to wait for one.
	alreadyApproved bool
	// releasable replays an engine that says the calling credential may answer
	// what it stages itself.
	releasable bool
}

func (r *recordingApprovals) StageCall(_ context.Context, in StageRequest) (ids.ApprovalID, bool, error) {
	r.staged = append(r.staged, in)
	return ids.New[ids.ApprovalKind](), r.alreadyApproved, nil
}

func (r *recordingApprovals) StageVolumeRelease(_ context.Context, in VolumeReleaseRequest) (ids.ApprovalID, bool, error) {
	r.steppedUp = append(r.steppedUp, in)
	switch {
	case r.stepUpErr != nil:
		return ids.ApprovalID{}, false, r.stepUpErr
	case r.stepUpDeclined:
		return ids.ApprovalID{}, false, nil
	default:
		return ids.New[ids.ApprovalKind](), true, nil
	}
}

func (r *recordingApprovals) ReleasableByCaller(context.Context, StageRequest) bool {
	return r.releasable
}

func (r *recordingApprovals) Redeem(_ context.Context, id ids.ApprovalID, tool, hash string) (int64, bool, error) {
	if r.redeemErr != nil {
		return 0, false, r.redeemErr
	}
	r.redeemed = append(r.redeemed, fmt.Sprintf("%s %s %s", id, tool, hash))
	return 0, false, nil
}

// nativeRecord stamps the authority a local system of record always carries.
// datasource.NewRecord does it for the real provider. refuseStagingElsewhere
// reads this flag, so an unstamped fixture is refused and the spec would test
// the external-target path instead of the native one.
func nativeRecord(rec datasource.Record) datasource.Record {
	rec.Freshness.Authoritative = true
	return rec
}

// agentCtx is automatic work: an agent with no passport, which a confirm-first
// call still stages for.
func agentCtx() context.Context {
	ctx := principal.WithWorkspaceID(context.Background(), ids.NewV7())
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalAgent, ID: "agent:t", OnBehalfOf: ids.NewV7(),
		Scopes: principal.NewScopeSet(principal.ScopeWrite),
	})
}

// passportCtx is an agent working on a colleague's passport, which acts as that
// colleague (principal.ActsAsItsUser).
func passportCtx() context.Context {
	ctx := agentCtx()
	p, _ := principal.Actor(ctx)
	p.PassportID = ids.NewV7()
	return principal.WithActor(ctx, p)
}
