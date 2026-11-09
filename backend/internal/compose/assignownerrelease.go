// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Releasing a reassignment at scale (AUTO-T07's 🟡 branch).
//
// An automation that reassigns one record writes straight through the provider;
// one that reassigns at scale stages instead and waits (assign_owner_tier.go's
// resolveAssignOwnerTier). The staging half shipped and the release half did
// not, so approving the card spent a human's decision on nothing: no owner
// moved, and the run that raised it stayed in requires_approval permanently.
//
// The effect performs the same write the 🟢 branch performs — same provider,
// same ref, same patch — because a reassignment a human released must not be a
// second spelling of a reassignment an automation made. What the approval adds
// is the human, and the human is on the decision's own audit row.
//
// Two things differ, both on purpose. The 🟢 branch stamps Source "system"; this
// stamps "system:assign-owner-release", so the provenance column can tell a
// reassignment that ran on its own from one a human had to release first. And
// this write carries IfVersion, which the 🟢 branch has no use for: it writes
// immediately, while a release writes against a record that has been sitting in
// an inbox. Both are distinctions worth keeping, not accidents of two code paths.

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

// assignOwnerReleaseActor is the provenance the released write carries. The
// owner change came from an automation's rule, not from somebody typing it, and
// a later human edit must still read as stronger than it.
const assignOwnerReleaseActor = "system:assign-owner-release"

// runSettleTimeout bounds settling a run after a remote write. The step
// is detached from the request, because the write it follows cannot be undone.
const runSettleTimeout = 30 * time.Second

// assignOwnerReleaseEffect executes an approved at-scale reassignment and
// finishes its run. The write carries the approval's version pin.
//
// With recordUpdaterTx, redemption, write and run completion share one
// transaction. A failed write leaves the approval unconsumed, so deciding again
// re-drives it.
//
// Any other provider is another system, so the approval is redeemed before the
// write is sent. A failed or unanswered write is never re-sent. The run is
// marked failed, and the decider learns whether the change may have landed.
func assignOwnerReleaseEffect(svc *approvals.Service, provider datasource.SystemOfRecordProvider, db *database.DB) approvals.ApprovedEffect {
	return func(ctx context.Context, approvalID ids.ApprovalID, proposedChange json.RawMessage, diffHash string) error {
		entityType, entityID, err := svc.StagedTarget(ctx, approvalID)
		if err != nil {
			return fmt.Errorf("compose: the record an approved reassignment names: %w", err)
		}
		decider, ok := principal.Actor(ctx)
		if !ok {
			return errors.New("compose: assign_owner release without a deciding principal")
		}
		// The redemption runs as the decider and the write as the system actor.
		execCtx := principal.WithActor(ctx, principal.Principal{
			Type:       principal.PrincipalSystem,
			ID:         assignOwnerReleaseActor,
			UserID:     decider.UserID,
			OnBehalfOf: decider.UserID,
		})
		release := ownerRelease{
			svc: svc, db: db, approvalID: approvalID, diffHash: diffHash,
			// The patch is handed over as staged. The provider owns which fields
			// an entity type accepts, so compose does not re-encode it.
			in: datasource.UpdateInput{
				Ref:    datasource.EntityRef{Type: datasource.EntityType(entityType), ID: entityID},
				Patch:  proposedChange,
				Source: assignOwnerReleaseActor,
			},
		}
		if local, ok := provider.(recordUpdaterTx); ok {
			return releaseInOneTransaction(ctx, execCtx, release, local)
		}
		return releaseThenWrite(ctx, execCtx, release, provider)
	}
}

// ownerRelease is one approved reassignment on its way to the record.
type ownerRelease struct {
	svc        *approvals.Service
	db         *database.DB
	approvalID ids.ApprovalID
	diffHash   string
	in         datasource.UpdateInput
}

// releaseInOneTransaction redeems, writes and completes the run in one
// transaction.
func releaseInOneTransaction(ctx, execCtx context.Context, r ownerRelease, local recordUpdaterTx) error {
	write, err := local.PrepareUpdateTx(execCtx, r.in.Ref.Type)
	if err != nil {
		return fmt.Errorf("compose: preparing the approved reassignment: %w", err)
	}
	return r.svc.RedeemAndApplyPinned(ctx, r.approvalID, string(workflow.ActionAssignOwner), r.diffHash,
		func(tx pgx.Tx, version int64, pinned bool) error {
			if _, err := write(execCtx, tx, r.pinnedTo(version, pinned)); err != nil {
				return fmt.Errorf("compose: applying the approved reassignment: %w", err)
			}
			return automation.CompleteApprovedRunTx(execCtx, tx, r.approvalID)
		})
}

// releaseThenWrite spends the approval, then sends the write to a system whose
// transaction it cannot join.
func releaseThenWrite(ctx, execCtx context.Context, r ownerRelease, provider datasource.SystemOfRecordProvider) error {
	version, pinned, err := r.svc.Redeem(ctx, r.approvalID, string(workflow.ActionAssignOwner), r.diffHash)
	if err != nil {
		return err
	}
	if _, err := provider.Update(execCtx, r.pinnedTo(version, pinned)); err != nil {
		return r.remoteWriteFailed(execCtx, err)
	}
	// The change has landed, so a cancelled request must not leave the run
	// waiting, and a failure here must not read as "did not run".
	runCtx, cancel := context.WithTimeout(context.WithoutCancel(execCtx), runSettleTimeout)
	defer cancel()
	if err := automation.CompleteApprovedRun(runCtx, r.db, r.approvalID); err != nil {
		return fmt.Errorf("compose: the approved reassignment landed and its run was not completed: %w: %w",
			approvals.ErrEffectFollowUpFailed, err)
	}
	return nil
}

// remoteWriteFailed records a write that failed after its approval was spent.
// A refusal changed nothing. Any other error may have come after the write
// landed, so it is reported as unconfirmed and never re-sent.
func (r ownerRelease) remoteWriteFailed(execCtx context.Context, cause error) error {
	failure := fmt.Errorf("compose: the system of record refused the approved reassignment: %w", cause)
	if !automation.WriteRefused(cause) {
		failure = fmt.Errorf("compose: the approved reassignment was sent and not confirmed: %w: %w",
			approvals.ErrEffectUnconfirmed, cause)
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(execCtx), runSettleTimeout)
	defer cancel()
	if err := automation.FailApprovedRun(ctx, r.db, r.approvalID, cause); err != nil {
		return errors.Join(failure, fmt.Errorf("compose: marking the reassignment's run failed: %w", err))
	}
	return failure
}

// pinnedTo is the release's input bound to the version the approval pinned.
func (r ownerRelease) pinnedTo(version int64, pinned bool) datasource.UpdateInput {
	in := r.in
	if pinned {
		in.IfVersion = &version
	}
	return in
}
